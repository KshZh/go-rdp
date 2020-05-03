package rdp

import (
	"testing"
	"time"
)

func TestHeap(t *testing.T) {
	eh := newEventHeap()
	numMsgs := 8
	msgs := make([]Message, numMsgs)
	events := make([]event, numMsgs)
	for i := 0; i < numMsgs; i++ {
		msgs[i].SeqNum = i
		events[i].msg = &msgs[i]
		events[i].expiration = time.Now()
	}
	// 下面这份代码是错误的，
	// event是一个局部于for块的变量，依次获取events[i]，
	// 拷贝到event中，供for块内使用，所以这份代码实际上
	// 在eh中放入了numMsgs个指向同一个对象的指针。
	// for _, event := range events {
	// 	eh.push(&event)
	// }
	for i := numMsgs - 1; i >= 0; i -= 2 {
		eh.push(&events[i])
	}
	for i := 0; i < numMsgs; i += 2 {
		eh.push(&events[i])
	}
	prev := eh.get(0, 0)
	if prev == nil || prev.msg.SeqNum != 0 {
		t.Error("")
	}
	for i := 1; i < numMsgs; i++ {
		e := eh.get(0, i)
		if e == nil || e.msg.SeqNum != i || !e.expiration.After(prev.expiration) {
			t.Error("")
		}
		prev = e
	}
	eh.remove(0, 3)
	if eh.get(0, 3) != nil {
		t.Error("")
	}
	prev = eh.pop()
	for eh.Len() > 0 {
		e := eh.pop()
		// t.Logf("%#v", e.msg)
		if !e.expiration.After(prev.expiration) {
			t.Error("")
		}
		prev = e
	}

	//
	msgs[2].ConnID = 2
	// 其他msg的ConnID保持为0。
	for i := 0; i < numMsgs; i++ {
		eh.push(&events[i])
	}
	eh.removeAll(0)
	if eh.Len() != 1 {
		t.Error("")
	}
	if eh.pop().msg.ConnID != 2 || eh.Len() != 0 {
		t.Error("")
	}
}
