# rdp, reliable datagram protocol

rdp是一个基于udp的可靠的应用层传输协议。

实现rdp的动机是在尝试学习分布式入门课程[cmu 15-440](https://www.synergylabs.org/courses/15-440/syllabus.html)时看到的作业[Project 1](https://github.com/cmu-440-f19/P1)，该project中第一步正是实现一个基于udp的应用层可靠传输协议lsp，是的，原名是lsp，即live sequence protocol，这里我把它实现了之后改名成了rdp，即reliable datagram protocol。

udp在ip上只是添加了基于端口的复用和分用以及差错检测功能，rdp在udp基础上实现可靠传输。
rdp的

project中只给出了rdp的接口定义、消息格式和测试用例，也就是文件client_api.go、server_api.go、lsp[1-5]_test.go、message.go、params.go、checksum.go这些文件，然后我自己实现了这些接口，即

我是用原project给出的测试用例验证了实现的正确性，对于性能并没有去测试，比如对比同样是可靠传输协议的tcp的性能，一个原因也是自己对如何测试性能没有思路。
