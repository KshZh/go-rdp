package rdp

import (
	"testing"
)

func TestSortedList(t *testing.T) {
	l := newSortedList()
	numMsgs := 50
	msgs := make([]Message, numMsgs)
	for i := 0; i < numMsgs; i++ {
		// SeqNum 0用于建立连接，数据交换用到的序列号从1开始。
		msgs[i].SeqNum = i + 1
	}
	// 先将偶数序列号的Message顺序插入链表。
	for i := 1; i < numMsgs; i += 2 {
		l.push(&msgs[i])
	}
	if l.pop() != nil {
		t.Error("")
	}
	// 然后将奇数序列号的Message逆序插入链表。
	for i := numMsgs - 2; i >= 0; i -= 2 {
		l.push(&msgs[i])
	}
	// 验证链表中所有结点都是按照Message.SeqNum升序排好的。
	i := 1
	for {
		msg := l.pop()
		if msg == nil {
			break
		}
		// t.Logf("%#v", *msg)
		if msg.SeqNum != i {
			t.Error("")
		}
		i++
	}
	if i != numMsgs+1 {
		t.Error("")
	}
}
