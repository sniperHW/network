package netgo

import (
	"github.com/xtaci/kcp-go/v5"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

type kcpSocket struct {
	userData       atomic.Value
	packetReceiver PacketReceiver
	conn           *kcp.UDPSession
	closeOnce      sync.Once
}

func (kc *kcpSocket) LocalAddr() net.Addr {
	return kc.conn.LocalAddr()
}

func (kc *kcpSocket) RemoteAddr() net.Addr {
	return kc.conn.RemoteAddr()
}

func (kc *kcpSocket) SetUserData(ud interface{}) {
	kc.userData.Store(userdata{
		data: ud,
	})
}

func (kc *kcpSocket) GetUserData() interface{} {
	if ud, ok := kc.userData.Load().(userdata); ok {
		return ud
	} else {
		return nil
	}
}

func (kc *kcpSocket) GetUnderConn() interface{} {
	return kc.conn
}

func (kc *kcpSocket) Close() {
	kc.closeOnce.Do(func() {
		runtime.SetFinalizer(kc, nil)
		kc.conn.Close()
	})
}

func (kc *kcpSocket) Send(data []byte, deadline ...time.Time) (int, error) {
	d := time.Time{}
	if len(deadline) > 0 && !deadline[0].IsZero() {
		d = deadline[0]
	}

	if err := kc.conn.SetWriteDeadline(d); err != nil {
		return 0, err
	} else {
		return kc.conn.Write(data)
	}

}

func (kc *kcpSocket) Recv(deadline ...time.Time) ([]byte, error) {
	if len(deadline) > 0 && !deadline[0].IsZero() {
		return kc.packetReceiver.Recv(kc.conn, deadline[0])
	} else {
		return kc.packetReceiver.Recv(kc.conn, time.Time{})
	}
}

func NewKcpSocket(conn *kcp.UDPSession, packetReceiver ...PacketReceiver) Socket {
	s := &kcpSocket{
		conn: conn,
	}

	if len(packetReceiver) == 0 || packetReceiver[0] == nil {
		s.packetReceiver = &defaultPacketReceiver{}
	} else {
		s.packetReceiver = packetReceiver[0]
	}

	runtime.SetFinalizer(s, func(s *kcpSocket) {
		s.Close()
	})

	return s
}
