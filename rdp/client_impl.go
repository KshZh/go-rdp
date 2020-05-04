// Contains the implementation of a RDP client.

package rdp

import (
	"encoding/json"
	"errors"

	// "log"
	"time"

	"github.com/KshZh/go-rdp/rdpnet"
)

type client struct {
	// TODO: implement this!
	connID int
	seqNum int
	conn   *rdpnet.UDPConn
	params *Params // 一些配置参数。

	timer         *time.Timer      // 计时器，其过期时间设置为事件堆中最先过期的事件的过期时间，所以当插入删除更新导致堆的调整时，那么记得要重置这个计时器。
	eventHeap     *eventHeap       // 事件堆。
	sendQueue     *restrictedQueue // 发送队列。
	receiveBuffer *sortedList      // 接收缓冲区。

	writeChan         chan *Message
	readChan          chan *Message
	ch                chan *Message
	connectionActived chan struct{}

	exit    chan struct{}
	closed  chan struct{}
	losted  chan struct{}
	closing chan chan error
}

// NewClient creates, initiates, and returns a new client. This function
// should return after a connection with the server has been established
// (i.e., the client has received an Ack message from the server in response
// to its connection request), and should return a non-nil error if a
// connection could not be made (i.e., if after K epochs, the client still
// hasn't received an Ack message from the server in response to its K
// connection requests).
//
// hostport is a colon-separated string identifying the server's host address
// and port number (i.e., "localhost:9999").
func NewClient(hostport string, params *Params) (Client, error) {
	cli := &client{
		connID: -1,
		seqNum: 0,
		params: params,

		timer:         time.NewTimer(time.Duration(time.Second) * 100),
		eventHeap:     newEventHeap(),
		sendQueue:     newRestrictedQueue(DefaultContainerSize, params.WindowSize, params.MaxUnackedMessages),
		receiveBuffer: newSortedList(),

		writeChan:         make(chan *Message),
		readChan:          make(chan *Message),
		ch:                make(chan *Message),
		connectionActived: make(chan struct{}),

		exit:    make(chan struct{}),
		losted:  make(chan struct{}, 1),
		closed:  make(chan struct{}, 1),
		closing: make(chan chan error),
	}
	cli.timer.Stop()
	// 解析HOST:PORT，可能需要对HOST做DNS查询，如果host不是IP地址的话。
	raddr, err := rdpnet.ResolveUDPAddr("udp", hostport)
	if err != nil {
		return nil, err
	}
	// 并没有像TCP一样三次握手建立连接，只是标记一下，当读写这个连接时，写到这里指定的HOST:PORT或从这里指定的HOST:PORT读。
	cli.conn, err = rdpnet.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, err
	}
	// 开启读写线程。
	go cli.readLoop()
	go cli.mainLoop()
	// 应用层开始尝试建立连接。
	cli.writeChan <- NewConnect()
	select {
	case <-cli.connectionActived:
		return cli, nil
	case <-time.After(time.Millisecond * time.Duration(params.EpochMillis*params.EpochLimit)):
	}
	// 建立连接失败，关闭读写线程。
	close(cli.exit)
	return nil, errors.New("connect fails")
}

func (c *client) ConnID() int {
	return c.connID
}

func (c *client) Read() ([]byte, error) {
	select {
	case msg := <-c.readChan:
		return msg.Payload, nil
	case <-c.closed:
		c.closed <- struct{}{} // 用于通知下一个Read()或Write()调用。
		return nil, errors.New("closed")
	case <-c.losted:
		c.losted <- struct{}{}
		return nil, errors.New("losted")
	}
}

func (c *client) Write(payload []byte) error {
	select {
	// 让mainLoop去写c.seqNum，如果在这里写的话，而mainLoop又会去读c.seqNum，这就有竞争条件了。
	// 而c.connID是只读的，所以可以直接读取后填充。
	case c.writeChan <- NewData(c.connID, -1, payload):
		return nil
	case <-c.closed:
		c.closed <- struct{}{}
		return errors.New("closed")
	case <-c.losted:
		c.losted <- struct{}{}
		return errors.New("losted")
	}
}

func (c *client) Close() error {
	// https://talks.golang.org/2013/advconc.slide#26
	errCh := make(chan error)
	select {
	case c.closing <- errCh:
		return <-errCh
	case <-c.closed:
		c.closed <- struct{}{}
		return errors.New("closed")
	case <-c.losted:
		c.losted <- struct{}{}
		return errors.New("losted")
	}
}

// 因为用的是阻塞I/O，所以要分两个I/O线程，一个读一个写，
// 否则只用一个线程，读阻塞就不能写，写阻塞就不能读，无法利用udp全双工的特性。
// 因为读大概率会阻塞，所以读单独一个线程，写不容易阻塞，所以写线程可以执行其它任务。
func (c *client) readLoop() {
	// for-select loop.
	readBytes := make([]byte, DefaultBufferSize)
	for {
		select {
		case <-c.exit:
			return
		default:
			// TODO
			// 这里应该设置超时，这样协议退出时，该线程才能最终返回退出。
			// 不过因为这里退出时，往往进程也快要退出了，所以应该没什么关系。
			n, err := c.conn.Read(readBytes)
			if err != nil {
				// log.Fatal(err)
				continue
			}
			var msg Message
			err = json.Unmarshal(readBytes[:n], &msg)
			if err != nil {
				continue
			}
			c.ch <- &msg
		}
	}
}

// 因为写线程一般不会阻塞，因为只要把数据写入内核缓冲区即可，只要内核缓冲区
// 有空间，就可以马上写（至于写到网卡，则由DMA异步完成，CPU只需要发出指令即可，不需要CPU参与拷贝数据），
// 而读的话，一般不知道对端什么时候给出响应，而且也不知道网络状况如何，可能丢包、延迟等，
// 所以读更容易阻塞，因此读线程专门读，写线程还可以做别的事情。
func (c *client) mainLoop() {
	// readChan - x - q1
	// writeChan1 - y - q2
	var readChan, writeChan1, writeChan2 chan *Message // 默认初始化为nil。
	// writeChan2必须是buffered channel，大小至少为1，才能让mainLoop不死锁。
	writeChan2 = make(chan *Message, DefaultBufferedChannelSize)
	var x, y *Message
	q1, q2 := newQueue(DefaultContainerSize), newQueue(DefaultContainerSize)

	var errCh chan error
	var closed, losted bool

	// for-select loop.
	for {
		if (closed || losted) && c.sendQueue.isEmpty() && c.receiveBuffer.isEmpty() {
			// 仅当所有pending消息都发出并被ack（即发送队列为空）和所有接收的消息都被读取后，系统才退出。
			close(c.exit) // 关闭该系统的所有线程。
			if closed {
				errCh <- nil // 通知Close()调用。
				c.closed <- struct{}{}
			} else {
				c.losted <- struct{}{}
			}
			return
		}
		if readChan == nil && !q1.isEmpty() {
			readChan = c.readChan
			x = q1.pop().(*Message)
		}
		if writeChan1 == nil && !q2.isEmpty() {
			writeChan1 = writeChan2
			y = q2.pop().(*Message)
		}
		select {
		case <-c.exit:
			return
		case errCh = <-c.closing:
			closed = true

		case readChan <- x:
			// log.Printf("cli[%d]: return %v", c.connID, x)
			// disable readChan.
			// 因为读写nil channel会永久阻塞，而select永远不会选中永久阻塞的case，
			// 所以可以避免将nil写入c.readChan。
			readChan = nil
		case writeChan1 <- y:
			writeChan1 = nil

		case <-c.timer.C:
			// 堆顶的事件过期。
			e := c.eventHeap.top()
			switch e.t {
			case heartbeat:
				// writeChan2即使是buffered channel，当满的时候写入也会死锁。
				// writeChan2 <- e.msg
				q2.push(e.msg)
			case trace:
				// 超时重传。
				q2.push(e.msg)
				// log.Printf("cli[%d]: resent %v", c.connID, e.msg)
			case lost:
				// 连接lost了，关闭连接。
				losted = true
				// log.Printf("cli[%d]: lost", c.connID)
			}
		case msg := <-c.ch:
			// log.Printf("cli[%d]: receive %v", c.connID, msg)
			if c.connID != -1 {
				// 收到对端发来的消息，表明对端还“活着”，重置堆中的lost事件的过期时间。
				c.eventHeap.updateExpiration(c.connID, lostSeqNum, time.Now().Add(time.Duration(c.params.EpochLimit*c.params.EpochMillis)*time.Millisecond))
			}
			// 收集读线程读到的消息，这里集中串行对收到的消息进行处理。
			switch msg.Type {
			case MsgAck:
				if msg.SeqNum == 0 {
					if c.connID == -1 {
						// 对MsgConnect的响应。
						c.connID = msg.ConnID
						c.sendQueue.ack(msg.SeqNum)
						// log.Printf("cli[%d] connection actived", msg.ConnID)
						// 连接建立后，在堆中放入两个事件，一个是heartbeat一个是lost。
						c.eventHeap.push(&event{
							t:           heartbeat,
							msg:         NewAck(c.connID, heartbeatSeqNum),
							currBackOff: 0,
							expiration:  time.Now().Add(time.Duration(c.params.EpochMillis) * time.Millisecond),
						})
						c.eventHeap.push(&event{
							t:           lost,
							msg:         NewAck(c.connID, lostSeqNum),
							currBackOff: 0,
							expiration:  time.Now().Add(time.Duration(c.params.EpochLimit*c.params.EpochMillis) * time.Millisecond),
						})
						// 通知NewClient()线程。
						c.connectionActived <- struct{}{}
						// 从事件堆中删除建立连接消息对应的超时事件。
						c.eventHeap.remove(0, 0)
					} else {
						// heartbeat，上面已经重置过lost事件了，这里就不需要做什么了。
					}
				} else {
					// 对MsgData的ack。
					if c.sendQueue.ack(msg.SeqNum) {
						// 从事件堆中删除该消息对应的超时事件。
						c.eventHeap.remove(msg.ConnID, msg.SeqNum)
						// 窗口中腾出了空间，看发送队列有没有可以发送的。
						if msg := c.sendQueue.get(); msg != nil {
							q2.push(msg)
						}
						// i, j, k, h := c.sendQueue.debug()
						// log.Printf("cli[%d]: srv had ack %v, total: %d, numSent: %d, numAcked: %d, numCanSend: %d", c.connID, msg, i, j, k, h)
					} else {
						// log.Printf("cli[%d]: duplicate %v", c.connID, msg)
					}
				}
			case MsgData:
				// 无论消息收到过没有，都必须ack。
				// 对于未收到过的消息，自然需要ack，对于重复的消息，也要ack，不然对端还会继续超时重传。
				// 因为不需要超时重传ack，所以直接发送ack，不放入发送队列中了。
				q2.push(NewAck(msg.ConnID, msg.SeqNum))
				if c.receiveBuffer.push(msg) {
					for {
						if msg := c.receiveBuffer.pop(); msg != nil {
							q1.push(msg)
						} else {
							break
						}
					}
					// log.Printf("cli[%d]: insert into receiveBuffer %v", c.connID, msg)
				} else {
					// log.Printf("cli[%d]: duplicate %v", c.connID, msg)
				}
			}
		case msg := <-c.writeChan:
			// 将写入c.queue的操作从客户端调用c.Write()的线程
			// 转移到这里，串行写操作，这样就不必处理并发写的问题。
			// 注意这里肯定要先放入发送队列中，而不是直接发送，这样才能受到滑动窗口的限制。
			msg.SeqNum = c.seqNum
			c.sendQueue.push(msg)
			c.seqNum++
			for {
				if msg := c.sendQueue.get(); msg != nil {
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
			n, err := c.conn.Write(bytes)
			if err != nil || n != len(bytes) {
				// log.Fatal(err)
			}
			// log.Printf("cli[%d]: send %v", c.connID, msg)
			switch msg.Type {
			case MsgConnect:
				e := c.eventHeap.get(msg.ConnID, msg.SeqNum)
				if e == nil {
					c.eventHeap.push(&event{
						t:           trace,
						msg:         msg,
						currBackOff: 0,
						expiration:  time.Now().Add(time.Duration(c.params.EpochMillis) * time.Millisecond),
					})
				} else {
					c.eventHeap.updateExpiration(msg.ConnID, msg.SeqNum, time.Now().Add(time.Duration(c.params.EpochMillis)*time.Millisecond))
				}
				resetTimer(c.timer, c.eventHeap.top().expiration)
			case MsgData:
				e := c.eventHeap.get(msg.ConnID, msg.SeqNum)
				if e == nil {
					c.eventHeap.push(&event{
						t:           trace,
						msg:         msg,
						currBackOff: 0,
						expiration:  time.Now().Add(time.Duration(c.params.EpochMillis) * time.Millisecond),
					})
				} else {
					if e.currBackOff < c.params.MaxBackOffInterval {
						if e.currBackOff == 0 {
							e.currBackOff = 1
						} else {
							e.currBackOff *= 2 // 超时，协议假设网络拥塞，那么指数退避，避免注入过多流量，导致网络一直拥塞。
							if e.currBackOff > c.params.MaxBackOffInterval {
								e.currBackOff = c.params.MaxBackOffInterval
							}
						}
					}
					c.eventHeap.updateExpiration(msg.ConnID, msg.SeqNum, time.Now().Add(time.Duration((1+e.currBackOff)*c.params.EpochMillis)*time.Millisecond))
				}
				resetTimer(c.timer, c.eventHeap.top().expiration)
			case MsgAck:
				if msg.SeqNum == 0 {
					// 发送的是heartbeat，重置heartbeat事件。
					c.eventHeap.updateExpiration(c.connID, heartbeatSeqNum, time.Now().Add(time.Duration(c.params.EpochMillis)*time.Millisecond))
					resetTimer(c.timer, c.eventHeap.top().expiration)
				}
			}
		}
	}
}

func resetTimer(timer *time.Timer, expiration time.Time) {
	// Until returns the duration until t. It is shorthand for t.Sub(time.Now()).
	delay := time.Until(expiration)
	if delay < 0 {
		timer.Reset(time.Duration(0))
	} else {
		timer.Reset(delay)
	}
}
