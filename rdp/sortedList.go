package rdp

import "container/list"

// 可以用平衡搜索树替代链表，保持有序且查找插入删除时间复杂度为O(logN)。

// https://golang.org/pkg/container/list/

type sortedList struct {
	lastReturnedSeqNum int
	lst                *list.List
}

// 工厂。
func newSortedList() *sortedList {
	return &sortedList{lst: list.New()}
}

// O(N).
func (l *sortedList) push(msg *Message) bool {
	if msg.SeqNum <= l.lastReturnedSeqNum {
		return false
	}
	for e := l.lst.Front(); e != nil; e = e.Next() {
		val := e.Value.(*Message)
		if val.SeqNum == msg.SeqNum {
			// 链表中已存在，不再插入。
			return false
		}
		if val.SeqNum > msg.SeqNum {
			l.lst.InsertBefore(msg, e)
			return true
		}
	}
	l.lst.PushBack(msg)
	return true
}

func (l *sortedList) pop() *Message {
	if l.lst.Front() == nil {
		return nil
	}
	msg := l.lst.Front().Value.(*Message)
	if msg.SeqNum == l.lastReturnedSeqNum+1 {
		l.lst.Remove(l.lst.Front())
		l.lastReturnedSeqNum++
		return msg
	}
	return nil
}

func (l *sortedList) isEmpty() bool {
	return l.lst.Front() == nil
}
