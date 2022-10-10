package netgo

//go test -race -covermode=atomic -v -coverprofile=coverage.out -run=.
//go tool cover -html=coverage.out
import (
	"crypto/sha1"
	gorilla "github.com/gorilla/websocket"
	"github.com/xtaci/kcp-go/v5"
	"golang.org/x/crypto/pbkdf2"
	"log"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestKcpSocket(t *testing.T) {
	key := pbkdf2.Key([]byte("demo pass"), []byte("demo salt"), 1024, 32, sha1.New)
	block, _ := kcp.NewAESBlockCrypt(key)

	var (
		listener *kcp.Listener
		err      error
	)

	if listener, err = kcp.ListenWithOptions("127.0.0.1:12345", block, 10, 3); err == nil {
		go func() {
			for {
				conn, err := listener.AcceptKCP()
				if err != nil {
					return
				}

				log.Println("TestKcpSocket:on new client")

				s := NewKcpSocket(conn)
				go func() {
					for {
						packet, err := s.Recv()
						if nil != err {
							log.Println("TestKcpSocket:server recv err:", err)
							break
						}
						s.Send(packet)
					}
					s.Close()
				}()
			}
		}()
	} else {
		log.Fatal(err)
	}

	{

		key := pbkdf2.Key([]byte("demo pass"), []byte("demo salt"), 1024, 32, sha1.New)
		block, _ := kcp.NewAESBlockCrypt(key)
		// dial to the echo server
		if conn, err := kcp.DialWithOptions("127.0.0.1:12345", block, 10, 3); err == nil {
			s := NewKcpSocket(conn)

			s.Send([]byte("hello"))

			packet, err := s.Recv()

			log.Println("TestKcpSocket:client", string(packet), err)

			s.Close()
		} else {
			log.Fatal(err)
		}
	}

	listener.Close()
}

func TestWebSocket(t *testing.T) {
	{

		tcpAddr, _ := net.ResolveTCPAddr("tcp", "localhost:8110")

		listener, _ := net.ListenTCP("tcp", tcpAddr)

		upgrader := &gorilla.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		}

		http.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
			conn, _ := upgrader.Upgrade(w, r, nil)

			conn.SetPingHandler(func(appData string) error {
				log.Println("TestWebSocket:on ping")
				conn.WriteMessage(gorilla.PongMessage, []byte(appData))
				return nil
			})

			log.Println("TestWebSocket:on client")
			s := NewWebSocket(conn)
			go func() {
				for {
					packet, err := s.Recv()
					if nil != err {
						log.Println("TestWebSocket:server recv err:", err)
						break
					}
					s.Send(packet)
				}
				s.Close()
			}()
		})

		go func() {
			http.Serve(listener, nil)
		}()

		{

			u := url.URL{Scheme: "ws", Host: "localhost:8110", Path: "/echo"}
			dialer := gorilla.DefaultDialer

			conn, _, _ := dialer.Dial(u.String(), nil)

			respChan := make(chan struct{})
			s := NewWebSocket(conn)

			conn.WriteMessage(gorilla.PingMessage, []byte("hello"))

			conn.SetPongHandler(func(appData string) error {
				log.Println("TestWebSocket:on pong")
				close(respChan)
				return nil
			})

			go func() {
				//触发recv接收Pong
				s.Recv()
			}()

			<-respChan

			s.Close()
		}

		{
			u := url.URL{Scheme: "ws", Host: "localhost:8110", Path: "/echo"}
			dialer := gorilla.DefaultDialer

			conn, _, _ := dialer.Dial(u.String(), nil)

			s := NewWebSocket(conn)

			s.Send([]byte("hello"))

			packet, err := s.Recv()

			log.Println("TestWebSocket:client", string(packet), err)

			s.Close()
		}

		listener.Close()

	}
}

func TestAsynSocket(t *testing.T) {
	MaxSendBlockSize = 64
	{
		listener, serve, _ := ListenTCP("tcp", "localhost:8110", func(conn *net.TCPConn) {
			log.Println("TestAsynSocket: on client")
			NewAsynSocket(NewTcpSocket(conn), AsynSocketOption{
				AutoRecv:        true,
				AutoRecvTimeout: time.Second,
			}).SetCloseCallback(func(_ *AsynSocket, err error) {
				log.Println("TestAsynSocket: server closed err:", err)
			}).SetPacketHandler(func(as *AsynSocket, packet interface{}) {
				log.Println("TestAsynSocket: server on packet", string(packet.([]byte)))
				if err := as.Send(packet); nil != err {
					as.Close(err)
					return
				}
			}).Recv(time.Now().Add(time.Second))
		})

		go serve()

		log.Println("here")

		dialer := &net.Dialer{}

		{
			conn, _ := dialer.Dial("tcp", "localhost:8110")
			okChan := make(chan struct{})

			as := NewAsynSocket(NewTcpSocket(conn.(*net.TCPConn)), AsynSocketOption{}).SetCloseCallback(func(_ *AsynSocket, err error) {
				log.Println("TestAsynSocket: client closed err:", err)
			}).SetPacketHandler(func(as *AsynSocket, packet interface{}) {
				log.Println("TestAsynSocket: client", string(packet.([]byte)))
				close(okChan)
			}).Recv()
			log.Println("TestAsynSocket: send", as.Send([]byte("hello")))
			<-okChan
			as.Close(nil)
		}

		log.Println("TestAsynSocket:-----------------------------")

		{
			conn, _ := dialer.Dial("tcp", "localhost:8110")
			okChan := make(chan struct{})
			as := NewAsynSocket(NewTcpSocket(conn.(*net.TCPConn)), AsynSocketOption{}).SetCloseCallback(func(_ *AsynSocket, err error) {
				log.Println("TestAsynSocket:client closed err:", err)
				close(okChan)
			}).Recv()
			<-okChan
			as.Close(nil)
		}

		listener.Close()
	}

	{

		log.Println("TestAsynSocket:-----------------------------2------")

		okChan := make(chan struct{})

		listener, serve, _ := ListenTCP("tcp", "localhost:8110", func(conn *net.TCPConn) {
			log.Println("TestAsynSocket:on client")
			i := 0
			NewAsynSocket(NewTcpSocket(conn), AsynSocketOption{}).SetCloseCallback(func(_ *AsynSocket, err error) {
				log.Println("TestAsynSocket:server closed err:", err)
			}).SetPacketHandler(func(as *AsynSocket, packet interface{}) {
				i = i + len(packet.([]byte))
				log.Println("TestAsynSocket:", i)
				if i == 100*5 {
					close(okChan)
				} else {
					as.Recv(time.Now().Add(time.Second))
				}
			}).Recv(time.Now().Add(time.Second))
		})

		go serve()

		dialer := &net.Dialer{}

		{
			conn, _ := dialer.Dial("tcp", "localhost:8110")
			as := NewAsynSocket(NewTcpSocket(conn.(*net.TCPConn)), AsynSocketOption{
				SendChanSize: 1000,
			}).SetCloseCallback(func(_ *AsynSocket, err error) {
				log.Println("TestAsynSocket:client closed err:", err)
			})

			for i := 0; i < 100; i++ {
				as.Send([]byte("hello"), time.Now().Add(time.Second))
			}

			as.Close(nil)

			<-okChan
		}

		listener.Close()

	}
}

func TestTCPSocket(t *testing.T) {

	{
		listener, serve, _ := ListenTCP("tcp", "localhost:8110", func(conn *net.TCPConn) {
			log.Println("TestTCPSocket:on client")
			s := NewTcpSocket(conn)
			go func() {
				for {
					packet, err := s.Recv(time.Now().Add(time.Second))
					if nil != err {
						log.Println("TestTCPSocket:server recv err:", err)
						break
					}
					s.Send(packet)
				}
				s.Close()
			}()
		})

		go serve()

		dialer := &net.Dialer{}

		{
			conn, _ := dialer.Dial("tcp", "localhost:8110")
			s := NewTcpSocket(conn.(*net.TCPConn))
			s.Send([]byte("hello"))
			packet, err := s.Recv()
			log.Println("TestTCPSocket:client", string(packet), err)
			s.Close()
		}

		{
			conn, _ := dialer.Dial("tcp", "localhost:8110")
			s := NewTcpSocket(conn.(*net.TCPConn))
			packet, err := s.Recv()
			log.Println("TestTCPSocket:client", string(packet), err)
			s.Close()

		}

		listener.Close()
	}
}