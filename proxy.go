package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"sync"
	"time"

	"tailscale.com/tsnet"
)

type ProxyConfig struct {
	Port     int
	Protocol string // "tcp" or "udp"
	Label    string
}

// SunshinePorts returns the verified hardware and control matrix from LizardByte spec
func SunshinePorts() []ProxyConfig {
	return []ProxyConfig{
		{Port: 47984, Protocol: "tcp", Label: "HTTPS GameStream Control"},
		{Port: 47989, Protocol: "tcp", Label: "HTTP GameStream Control"},
		{Port: 47990, Protocol: "tcp", Label: "Sunshine Web UI Admin"},
		{Port: 48010, Protocol: "tcp", Label: "RTSP Stream Session Control"},
		{Port: 47998, Protocol: "udp", Label: "Video Output Stream"},
		{Port: 47999, Protocol: "udp", Label: "Input Peripheral / Controller Link"},
		{Port: 48000, Protocol: "udp", Label: "Audio Sound Output Stream"},
		{Port: 48002, Protocol: "udp", Label: "Microphone Back-channel"},
	}
}

// StartServerProxy runs on Host PC: tsnet listener -> OS Local dial 127.0.0.1
func StartServerProxy(ctx context.Context, srv *tsnet.Server, internalLog *log.Logger, wg *sync.WaitGroup) {
	for _, p := range SunshinePorts() {
		wg.Add(1)
		if p.Protocol == "tcp" {
			go startServerTCP(ctx, srv, p, internalLog, wg)
		} else {
			go startServerUDP(ctx, srv, p, internalLog, wg)
		}
	}
}

// StartClientProxy runs on Client PC: OS Local listener 127.0.0.1 -> tsnet dial to Host PC
func StartClientProxy(ctx context.Context, srv *tsnet.Server, targetIP string, internalLog *log.Logger, wg *sync.WaitGroup) {
	for _, p := range SunshinePorts() {
		wg.Add(1)
		if p.Protocol == "tcp" {
			go startClientTCP(ctx, srv, targetIP, p, internalLog, wg)
		} else {
			go startClientUDP(ctx, srv, targetIP, p, internalLog, wg)
		}
	}
}

// --- OPTIMIZATIONS ---

var udpBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 65535)
		return &b
	},
}

func setTCPKeepAlive(c net.Conn) {
	if tcpConn, ok := c.(*net.TCPConn); ok {
		_ = tcpConn.SetKeepAlive(true)
		_ = tcpConn.SetKeepAlivePeriod(15 * time.Second)
	}
}

// dialWithBackoff retries a srv.Dial call with exponential backoff to handle mesh route delays.
func dialWithBackoff(ctx context.Context, srv *tsnet.Server, network, addr string, maxRetries int, logger *log.Logger) (net.Conn, error) {
	var conn net.Conn
	var err error
	delay := 100 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		conn, err = srv.Dial(ctx, network, addr)
		if err == nil {
			return conn, nil
		}
		logger.Printf("[Proxy] tsnet dial %s %s failed (attempt %d/%d): %v", network, addr, i+1, maxRetries, err)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}

		delay *= 2
		if delay > 1*time.Second {
			delay = 1 * time.Second
		}
	}
	return nil, err
}

// --- TCP CORE IMPLEMENTATION ---

func startServerTCP(ctx context.Context, srv *tsnet.Server, p ProxyConfig, logger *log.Logger, wg *sync.WaitGroup) {
	defer wg.Done()
	ln, err := srv.Listen("tcp", fmt.Sprintf(":%d", p.Port))
	if err != nil {
		logger.Printf("[Proxy] Failed to listen on tsnet TCP port %d: %v", p.Port, err)
		return
	}
	defer ln.Close()

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		srcConn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		go func(src net.Conn) {
			defer src.Close()
			dst, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", p.Port), 5*time.Second)
			if err != nil {
				logger.Printf("[Proxy] Failed to dial local Sunshine TCP port %d: %v", p.Port, err)
				return
			}
			defer dst.Close()

			setTCPKeepAlive(src)
			setTCPKeepAlive(dst)

			relayTCP(src, dst)
		}(srcConn)
	}
}

func startClientTCP(ctx context.Context, srv *tsnet.Server, targetIP string, p ProxyConfig, logger *log.Logger, wg *sync.WaitGroup) {
	defer wg.Done()
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p.Port))
	if err != nil {
		logger.Printf("[Proxy] Client layer failed to bind local OS TCP port %d: %v", p.Port, err)
		return
	}
	defer ln.Close()

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		srcConn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		go func(src net.Conn) {
			defer src.Close()
			
			// Exponential backoff to handle mesh route delays during initial Moonlight launch
			dst, err := dialWithBackoff(ctx, srv, "tcp", fmt.Sprintf("%s:%d", targetIP, p.Port), 5, logger)
			if err != nil {
				logger.Printf("[Proxy] Client failed to punch mesh route to server TCP %d: %v", p.Port, err)
				return
			}
			defer dst.Close()

			setTCPKeepAlive(src)
			setTCPKeepAlive(dst)

			relayTCP(src, dst)
		}(srcConn)
	}
}

func relayTCP(src, dst net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	copyAndClose := func(r, w net.Conn) {
		defer wg.Done()
		_, _ = io.Copy(w, r)
		if tcpConn, ok := w.(*net.TCPConn); ok {
			_ = tcpConn.CloseWrite()
		}
	}
	go copyAndClose(src, dst)
	go copyAndClose(dst, src)
	wg.Wait()
}

// --- UDP CORE IMPLEMENTATION ---

type udpSession struct {
	localConn  *net.UDPConn
	lastActive time.Time
}

func startServerUDP(ctx context.Context, srv *tsnet.Server, p ProxyConfig, logger *log.Logger, wg *sync.WaitGroup) {
	defer wg.Done()
	ln, err := srv.ListenPacket("udp", fmt.Sprintf(":%d", p.Port))
	if err != nil {
		logger.Printf("[Proxy] Failed to bind packet listener on tsnet UDP port %d: %v", p.Port, err)
		return
	}
	defer ln.Close()

	sessions := make(map[netip.AddrPort]*udpSession)
	var mu sync.Mutex

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				mu.Lock()
				now := time.Now()
				for addr, sess := range sessions {
					if now.Sub(sess.lastActive) > 30*time.Second {
						sess.localConn.Close()
						delete(sessions, addr)
					}
				}
				mu.Unlock()
			}
		}
	}()

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		bufPtr := udpBufPool.Get().(*[]byte)
		buf := *bufPtr

		n, raddr, err := ln.ReadFrom(buf)
		if err != nil {
			udpBufPool.Put(bufPtr)
			if ctx.Err() != nil {
				return
			}
			continue
		}

		netipAddr, err := netip.ParseAddrPort(raddr.String())
		if err != nil {
			udpBufPool.Put(bufPtr)
			continue
		}

		mu.Lock()
		sess, exists := sessions[netipAddr]
		if !exists {
			rUDPAddr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", p.Port))
			localConn, err := net.DialUDP("udp", nil, rUDPAddr)
			if err != nil {
				mu.Unlock()
				udpBufPool.Put(bufPtr)
				continue
			}
			sess = &udpSession{localConn: localConn, lastActive: time.Now()}
			sessions[netipAddr] = sess

			go func(netAddr net.Addr, c *net.UDPConn) {
				for {
					backBufPtr := udpBufPool.Get().(*[]byte)
					backBuf := *backBufPtr
					
					bn, err := c.Read(backBuf)
					if err != nil {
						udpBufPool.Put(backBufPtr)
						return
					}
					mu.Lock()
					sess.lastActive = time.Now()
					mu.Unlock()
					_, _ = ln.WriteTo(backBuf[:bn], netAddr)
					udpBufPool.Put(backBufPtr)
				}
			}(raddr, localConn)
		}
		sess.lastActive = time.Now()
		mu.Unlock()

		_, _ = sess.localConn.Write(buf[:n])
		udpBufPool.Put(bufPtr)
	}
}

func startClientUDP(ctx context.Context, srv *tsnet.Server, targetIP string, p ProxyConfig, logger *log.Logger, wg *sync.WaitGroup) {
	defer wg.Done()
	lUDPAddr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", p.Port))
	ln, err := net.ListenUDP("udp", lUDPAddr)
	if err != nil {
		logger.Printf("[Proxy] Client error listening on local OS UDP port %d: %v", p.Port, err)
		return
	}
	defer ln.Close()

	sessions := make(map[netip.AddrPort]net.Conn)
	var mu sync.Mutex

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		bufPtr := udpBufPool.Get().(*[]byte)
		buf := *bufPtr

		n, raddr, err := ln.ReadFrom(buf)
		if err != nil {
			udpBufPool.Put(bufPtr)
			if ctx.Err() != nil {
				return
			}
			continue
		}

		netipAddr, err := netip.ParseAddrPort(raddr.String())
		if err != nil {
			udpBufPool.Put(bufPtr)
			continue
		}

		mu.Lock()
		meshConn, exists := sessions[netipAddr]
		if !exists {
			// Using exponential backoff for UDP dial as well since tsnet may need time to establish route
			meshConn, err = dialWithBackoff(ctx, srv, "udp", fmt.Sprintf("%s:%d", targetIP, p.Port), 5, logger)
			if err != nil {
				mu.Unlock()
				udpBufPool.Put(bufPtr)
				continue
			}
			sessions[netipAddr] = meshConn

			go func(clientAddr net.Addr, mc net.Conn) {
				for {
					backBufPtr := udpBufPool.Get().(*[]byte)
					backBuf := *backBufPtr

					bn, err := mc.Read(backBuf)
					if err != nil {
						udpBufPool.Put(backBufPtr)
						return
					}
					_, _ = ln.WriteTo(backBuf[:bn], clientAddr)
					udpBufPool.Put(backBufPtr)
				}
			}(raddr, meshConn)
		}
		mu.Unlock()

		_, _ = meshConn.Write(buf[:n])
		udpBufPool.Put(bufPtr)
	}
}
