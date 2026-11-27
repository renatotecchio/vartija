//go:build linux

package arp

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

func htons(v uint16) uint16 { return v<<8 | v>>8 }

// sweep sends the requests on an AF_PACKET socket bound to the interface
// and reads the replies until Wait after the last request.
func sweep(ctx context.Context, at attachment, targets []netip.Addr, s *Scanner) (map[netip.Addr]net.HardwareAddr, error) {
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW|unix.SOCK_CLOEXEC, int(htons(etherTypeARP)))
	if err != nil {
		if errors.Is(err, unix.EPERM) {
			return nil, errors.New("raw socket not allowed: the container needs CAP_NET_RAW")
		}
		return nil, err
	}
	var closeOnce sync.Once
	closeFD := func() { closeOnce.Do(func() { unix.Close(fd) }) }
	defer closeFD()

	sll := &unix.SockaddrLinklayer{Protocol: htons(etherTypeARP), Ifindex: at.iface.Index}
	if err := unix.Bind(fd, sll); err != nil {
		return nil, err
	}
	// Short read timeout so the reader notices the deadline
	tv := unix.NsecToTimeval((200 * time.Millisecond).Nanoseconds())
	if err := unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tv); err != nil {
		return nil, err
	}

	found := map[netip.Addr]net.HardwareAddr{}
	var mu sync.Mutex
	done := make(chan struct{})
	var deadline time.Time
	var deadlineMu sync.Mutex
	setDeadline := func(t time.Time) { deadlineMu.Lock(); deadline = t; deadlineMu.Unlock() }
	getDeadline := func() time.Time { deadlineMu.Lock(); defer deadlineMu.Unlock(); return deadline }
	// Until the last request is out, the reader runs with no deadline
	setDeadline(time.Now().Add(24 * time.Hour))

	go func() {
		defer close(done)
		buf := make([]byte, 1500)
		for time.Now().Before(getDeadline()) && ctx.Err() == nil {
			n, _, err := unix.Recvfrom(fd, buf, 0)
			if err != nil {
				if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR) {
					continue
				}
				return
			}
			if ip, mac, ok := parseReply(buf[:n]); ok {
				mu.Lock()
				found[ip] = mac
				mu.Unlock()
			}
		}
	}()

	dst := &unix.SockaddrLinklayer{Protocol: htons(etherTypeARP), Ifindex: at.iface.Index, Halen: 6}
	copy(dst.Addr[:], broadcast)
	var sendErr error
send:
	for round := 0; round < s.Rounds; round++ {
		for _, t := range targets {
			if ctx.Err() != nil {
				break send
			}
			mu.Lock()
			_, answered := found[t]
			mu.Unlock()
			if answered || t == at.ip {
				continue
			}
			if err := unix.Sendto(fd, request(at.iface.HardwareAddr, at.ip, t), 0, dst); err != nil {
				sendErr = err
				break send
			}
			time.Sleep(s.Gap)
		}
	}
	setDeadline(time.Now().Add(s.Wait))
	<-done
	if sendErr != nil {
		return nil, sendErr
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	mu.Lock()
	defer mu.Unlock()
	return found, nil
}
