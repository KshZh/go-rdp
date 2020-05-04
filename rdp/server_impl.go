// Contains the implementation of a RDP server.

package rdp

import (
	"encoding/json"
	"errors"

	// "log"
	"strconv"
	"time"

	"github.com/KshZh/go-rdp/rdpnet"
)

type clientInfo struct {
	seqNum        int
	addr          *rdpnet.UDPAddr
	sendQueue     *restrictedQueue // 发送队列。
	receiveBuffer *sortedList      // 接收缓冲区。
}

type server struct {
	// TODO: Implement this!
	conn        *rdpnet.UDPConn
	clients     map[int]*clientInfo // connID -> clientInfo
	addr2connID map[string]int      // 记住哪些地址已经建立了连接。
	connIDGen   int                 // 线性生成connID。
	params      *Params             // 一些配置参数。
	timer       *time.Timer         // 计时器。
	eventHeap   *eventHeap          // 事件堆。

	writeChan chan *Message
	readChan  chan *Message
	ch        chan *Message
	addrChan  chan *rdpnet.UDPAddr

	exit      chan struct{}
	closed    chan struct{}
	losted    chan struct{}
	closing   chan chan error
	closeConn chan int
	xxChan    chan int
}

// NewServer creates, initiates, and returns a new server. This function should
// NOT block. Instead, it should spawn one or more goroutines (to handle things
// like accepting incoming client connections, triggering epoch events at
// fixed intervals, synchronizing events using a for-select loop like you saw in
// project 0, etc.) and immediately return. It should return a non-nil error if
// there was an error resolving or listening on the specified port number.
func NewServer(port int, params *Params) (Server, error) {
	s := &server{
		clients:     make(map[int]*clientInfo),
		addr2connID: make(map[string]int),
		connIDGen:   1,
		params:      params,
		timer:       time.NewTimer(time.Duration(time.Second) * 100),
		eventHeap:   newEventHeap(),
		readChan:    make(chan *Message),
		writeChan:   make(chan *Message),
		ch:          make(chan *Message),
		addrChan:    make(chan *rdpnet.UDPAddr),
		exit:        make(chan struct{}),
		closeConn:   make(chan int),
		closed:      make(chan struct{}, 1),
		losted:      make(chan struct{}, 1),
		closing:     make(chan chan error),
		xxChan:      make(chan int),
	}
	s.timer.Stop()
	// 解析HOST:PORT，可能需要对HOST做DNS查询，如果host不是IP地址的话。
	laddr, err := rdpnet.ResolveUDPAddr("udp", ":"+strconv.Itoa(port))
	if err != nil {
		return nil, err
	}
	s.conn, err = rdpnet.ListenUDP("udp", laddr)
	if err != nil {
		return nil, err
	}
	go s.readLoop()
	go s.mainLoop()
	return s, nil
}

func (s *server) Read() (int, []byte, error) {
	select {
	case connID := <-s.xxChan:
		return connID, nil, errors.New("closed || losted")
	case <-s.closed:
		s.closed <- struct{}{}
		return 0, nil, errors.New("closed")
	case <-s.losted:
		s.losted <- struct{}{}
		return 0, nil, errors.New("losted")
	case msg := <-s.readChan:
		return msg.ConnID, msg.Payload, nil
	}
}

func (s *server) Write(connId int, payload []byte) error {
	// 这里有一个竞争条件，也就是s.clients[connId]可能被mainLoop删掉了。
	// s.writeChan <- NewData(connId, s.clients[connId].seqNum, len(payload), payload, uint16(ByteArray2Checksum(payload)))
	// 所以这里先不填充seqNum，交给mainLoop去填充。
	select {
	case s.writeChan <- NewData(connId, -1, payload):
		return nil
	case <-s.closed:
		s.closed <- struct{}{}
		return errors.New("closed")
	case <-s.losted:
		s.losted <- struct{}{}
		return errors.New("losted")
	}
}

func (s *server) CloseConn(connId int) error {
	// 可能需要阻塞一小会，如果要求完全不阻塞，这里可以单独开一个协程。
	s.closeConn <- connId
	return nil
}

func (s *server) Close() error {
	errCh := make(chan error)
	select {
	case s.closing <- errCh:
		return <-errCh
	case <-s.closed:
		s.closed <- struct{}{}
		return errors.New("closed")
	case <-s.losted:
		s.losted <- struct{}{}
		return errors.New("losted")
	}
}

func (s *server) readLoop() {
	// for-select loop.
	buf := make([]byte, DefaultBufferSize)
	for {
		select {
		case <-s.exit:
			return
		default:
			n, raddr, err := s.conn.ReadFromUDP(buf)
			if err != nil {
				// // log.Fatal(err)
				continue
			}
			var msg Message
			err = json.Unmarshal(buf[:n], &msg)
			if err != nil {
				continue
			}
			s.ch <- &msg
			if msg.Type == MsgConnect {
				s.addrChan <- raddr
			}
		}
	}
}

func (s *server) mainLoop() {
	// readChan - x - q1
	// writeChan1 - y - q2
	var readChan, writeChan1, writeChan2 chan *Message
	writeChan2 = make(chan *Message, DefaultBufferedChannelSize)
	var x, y *Message
	q1, q2 := newQueue(DefaultContainerSize), newQueue(DefaultContainerSize)

	var errCh chan error
	closed, losted := make(map[int]bool), make(map[int]bool) // connID -> true/false
	q3 := newQueue(DefaultContainerSize)
	var closeCalled bool
	var xxChan chan int
	var xx int

	// for-select loop.
	for {
		for connID := range closed {
			if _, ok := s.clients[connID]; !ok {
				// 有可能一个连接即closed又losted，这样其中慢执行的一者就可能出现空指针异常。
				delete(closed, connID)
				continue
			}
			if s.clients[connID].sendQueue.isEmpty() && s.clients[connID].receiveBuffer.isEmpty() {
				q3.push(connID)
				delete(s.addr2connID, s.clients[connID].addr.String())
				delete(s.clients, connID)
				delete(closed, connID) // 通过测试，在迭代容器时删除当前元素不会影响对后序元素的遍历。
				// 把该连接相关的事件从事件堆中删去。
				s.eventHeap.removeAll(connID)
			}
		}
		for connID := range losted {
			if _, ok := s.clients[connID]; !ok {
				delete(losted, connID)
				continue
			}
			if s.clients[connID].receiveBuffer.isEmpty() {
				// 既然连接已经丢失了，那么就不再把pending消息发出去了。
				q3.push(connID)
				delete(s.addr2connID, s.clients[connID].addr.String())
				delete(s.clients, connID)
				delete(losted, connID) // 通过测试，在迭代容器时删除当前元素不会影响对后序元素的遍历。
			}
		}
		if closeCalled && len(closed) == 0 && len(losted) == 0 {
			close(s.exit)
			errCh <- nil
			return
		}

		if xxChan == nil && !q3.isEmpty() {
			xxChan = s.xxChan
			xx = q3.pop().(int)
		}
		if readChan == nil && !q1.isEmpty() {
			readChan = s.readChan
			x = q1.pop().(*Message)
		}
		if writeChan1 == nil && !q2.isEmpty() {
			writeChan1 = writeChan2
			y = q2.pop().(*Message)
		}
		select {
		case <-s.exit:
			return
		case connID := <-s.closeConn:
			closed[connID] = true
		case errCh = <-s.closing:
			closeCalled = true
			for connID := range s.clients {
				closed[connID] = true
			}

		case xxChan <- xx:
			xxChan = nil
		case readChan <- x:
			readChan = nil
			// log.Printf("srv: return %v", x)
		case writeChan1 <- y:
			writeChan1 = nil

		case <-s.timer.C:
			// 堆顶的事件过期。
			e := s.eventHeap.top()
			if e == nil {
				// 在等待期间，事件对应的连接丢失或关闭了，事件也被从堆中删除，
				// 可能现在是个空堆，那么就不需要做什么。
				continue
			}
			switch e.t {
			case heartbeat:
				q2.push(e.msg)
			case trace:
				// 超时重传。
				q2.push(e.msg)
				// log.Printf("srv: resent %v", e.msg)
			case lost:
				// 连接lost了，关闭连接。
				// log.Printf("srv cli[%d]: lost", e.msg.ConnID)
				losted[e.msg.ConnID] = true
				// 因为连接丢失了，所以发送数据没有意义，所以现在就把堆中与该连接相关的事件删除。
				s.eventHeap.removeAll(e.msg.ConnID)
				// XXX
				// 和客户端不同，服务端维护多个客户端，堆中有多个客户端的lost事件，
				// 一个连接lost，可能还有其它连接要继续处理。
				// 重置计时器，如果堆不为空的话。
				if s.eventHeap.top() != nil {
					resetTimer(s.timer, s.eventHeap.top().expiration)
				}
			}
		case msg := <-s.ch:
			if _, ok := s.clients[msg.ConnID]; ok {
				// 收到对端发来的消息，表明对端还“活着”，重置堆中的lost事件的过期时间。
				s.eventHeap.updateExpiration(msg.ConnID, lostSeqNum, time.Now().Add(time.Duration(s.params.EpochLimit*s.params.EpochMillis)*time.Millisecond))
			}
			if msg.Type != MsgConnect {
				if _, ok := s.clients[msg.ConnID]; !ok {
					// 来自未建立连接的非MsgConnect消息，忽略。
					continue
				}
			}
			// log.Printf("srv: receive %v", msg)
			switch msg.Type {
			case MsgConnect:
				raddr := <-s.addrChan
				if connID, ok := s.addr2connID[raddr.String()]; ok {
					// 已建立过连接了，可能之前的ack没送达，再次响应ack，告诉客户端连接已建立。
					// log.Println("srv: conn had actived", raddr.String())
					q2.push(NewAck(connID, msg.SeqNum))
				} else {
					// log.Println("srv: ack connection request: ", s.connIDGen)
					cli := &clientInfo{
						seqNum:        1,
						addr:          raddr,
						receiveBuffer: newSortedList(),
						sendQueue:     newRestrictedQueue(DefaultContainerSize, s.params.WindowSize, s.params.MaxUnackedMessages),
					}
					s.clients[s.connIDGen] = cli
					s.addr2connID[raddr.String()] = s.connIDGen
					// 连接建立后，在堆中放入两个事件，一个是heartbeat一个是lost。
					s.eventHeap.push(&event{
						t:           heartbeat,
						msg:         NewAck(s.connIDGen, heartbeatSeqNum),
						currBackOff: 0,
						expiration:  time.Now().Add(time.Duration(s.params.EpochMillis) * time.Millisecond),
					})
					s.eventHeap.push(&event{
						t:           lost,
						msg:         NewAck(s.connIDGen, lostSeqNum),
						currBackOff: 0,
						expiration:  time.Now().Add(time.Duration(s.params.EpochLimit*s.params.EpochMillis) * time.Millisecond),
					})
					q2.push(NewAck(s.connIDGen, 0))
					s.connIDGen++
				}
			case MsgAck:
				if msg.SeqNum == 0 {
					// heartbeat，表明对端还“活着”，只是暂时没有数据要发送。
					// 重置堆中的lost事件的过期时间。上面已经做了，这里就不需要再重置了。
				} else {
					// 对MsgData的响应。
					if s.clients[msg.ConnID].sendQueue.ack(msg.SeqNum) {
						// 从事件堆中删除该消息对应的超时事件。
						s.eventHeap.remove(msg.ConnID, msg.SeqNum)
						if msg := s.clients[msg.ConnID].sendQueue.get(); msg != nil {
							q2.push(msg)
						}
						// log.Printf("srv: cli[%d] had ack %v", msg.ConnID, msg)
					} else {
						// log.Printf("srv: duplicate %v", msg)
					}
				}
			case MsgData:
				// 无论消息收到过没有，都必须ack。
				// 对于未收到过的消息，自然需要ack，对于重复的消息，也要ack，不然对端还会继续超时重传。
				// 因为不需要超时重传ack，所以直接发送ack，不放入发送队列中了。
				q2.push(NewAck(msg.ConnID, msg.SeqNum))
				if !s.clients[msg.ConnID].receiveBuffer.push(msg) {
					// log.Printf("srv: duplicate %v", msg)
				} else {
					for {
						if msg := s.clients[msg.ConnID].receiveBuffer.pop(); msg != nil {
							q1.push(msg)
						} else {
							break
						}
					}
					// log.Printf("srv: insert into receiveBuffer %v", msg)
				}
			}
		case msg := <-s.writeChan:
			if _, ok := s.clients[msg.ConnID]; !ok {
				// 该连接不存在，即未建立或已关闭或已丢失。
				continue
			}
			// 填充seqNum。
			msg.SeqNum = s.clients[msg.ConnID].seqNum
			s.clients[msg.ConnID].seqNum++
			// log.Printf("srv: push %v", msg)
			s.clients[msg.ConnID].sendQueue.push(msg)
			for {
				if msg := s.clients[msg.ConnID].sendQueue.get(); msg != nil {
					q2.push(msg)
				} else {
					break
				}
			}
		case msg := <-writeChan2:
			bytes, err := json.Marshal(*msg)
			if err != nil {
				// log.Fatal(err)
			}
			n, err := s.conn.WriteToUDP(bytes, s.clients[msg.ConnID].addr)
			if err != nil || n != len(bytes) {
				// log.Fatal(err)
			}
			// log.Printf("srv: send %v", msg)
			switch msg.Type {
			case MsgData:
				e := s.eventHeap.get(msg.ConnID, msg.SeqNum)
				if e == nil {
					s.eventHeap.push(&event{
						t:           trace,
						msg:         msg,
						currBackOff: 0,
						expiration:  time.Now().Add(time.Duration(s.params.EpochMillis) * time.Millisecond),
					})
				} else {
					if e.currBackOff < s.params.MaxBackOffInterval {
						if e.currBackOff == 0 {
							e.currBackOff = 1
						} else {
							e.currBackOff *= 2 // 超时，协议假设网络拥塞，那么指数退避，避免注入过多流量，导致网络一直拥塞。
							if e.currBackOff > s.params.MaxBackOffInterval {
								e.currBackOff = s.params.MaxBackOffInterval
							}
						}
					}
					s.eventHeap.updateExpiration(msg.ConnID, msg.SeqNum, time.Now().Add(time.Duration((1+e.currBackOff)*s.params.EpochMillis)*time.Millisecond))
				}
				resetTimer(s.timer, s.eventHeap.top().expiration)
			case MsgAck:
				if msg.SeqNum == 0 {
					// 发送的是heartbeat，重置heartbeat事件。
					s.eventHeap.updateExpiration(msg.ConnID, heartbeatSeqNum, time.Now().Add(time.Duration(s.params.EpochMillis)*time.Millisecond))
					resetTimer(s.timer, s.eventHeap.top().expiration)
				}
			}
		}
	}
}
