# README

## 简介

本项目是提供一个用户态的基础网络包，对标 golang 的标准包-- [net](https://pkg.go.dev/net)，以 tcp/ip 协议栈为例，其所有的协议栈实现都在用户态，通过零拷贝的方式将数据写到 NIC，可以极大的提升pps(packets per second)。

以上实现主要依赖于 f-stack 项目，该项目基于 dpdk 封装实现了 tcp/ip 协议栈，本项目在 f-stack 基础上使用 cgo 技术将其嵌入到 golang 项目中。

## dpdk 
在互联网爆发式增长的时代背景下，网络终端设备越来越多，服务器的网络处理能力愈发的重要。除了传统的 C/S 场景外，有很多云厂商或大型互联网公司，为了实现SDN网络基础架构，会基于通用操作系统（例如：Unix、Linux等）内核 原生的能力 或 搭载open vswitch、vrouter 等软件，通过 虚拟化、软件模拟方式来替代以前专用的硬件网络设备，以此来降低硬件成本。此时通用操作系统在历史发展过程中，为了方便用户应用而提供“优秀的”网络栈封装反而成为制约性能的瓶颈，尤其是在硬件网卡达到 1G 、10 G 级别时，很难发挥出网卡的全部能力。

以 Linux 为例，在发送网络包时应用进程要经历：

1. 发送数据从用户空间拷贝到内核空间;
2. IP 协议栈处理数据，拼装 skb(socket kernel buffer);
3. skb 指针加入到 Driver queue;
4. Driver 触发 DMA 操作，数据拷贝到 NIC ;
5. NIC 将报文数据发送到物理网络介质。

![ip packets porcessing](https://www.linuxjournal.com/files/linuxjournal.com/ufiles/imagecache/large-550px-centered/u1002061/11476f2.jpg)

在 step 1 时应用进程会从用户态陷入到内核态，此时需要进行一次上下文切换，进程进入阻塞状态，等待 kernel 唤醒。当数据包发送完成后 ，网卡设备会产生一个中断信号，kernel 捕获该信号后唤醒对应的进程，从内核态恢复到用户态，此时需要进行一次上下文切换。 总计*需要进行两次上下文切换、进行一次内存拷贝，在此期间内核可能处理多次中断*，因为所有应用共用一个网卡设备（待确认）。尤其时在高并发、高pps场景下，会频繁的触发中断，产生中断风暴，CPU 资源部分消耗在中断处理 和 进程的上下文切换中。

目前流行的 CPU 架构都是多核架构，通过提高核数来提升整体的算力，一般的应用不会限制自己要在哪个核上执行，当应用进程发生跨核迁移的时候也会消耗性能。此外，当多个内核读写同一片内存时会频繁触发 Cache write back ，以及跨内核的 Cache 同步操作；同时进程迁移到新的内核后，新内核的 Cache 可能尚未加载进程读写的数据，会造成 Cache Miss。尤其是在NUMA LOAD 的架构中，每一个内核都有一块绑定的“本地内存”，读写速度最快，读写非本地内存性能会有很大损耗。

还有内存地址转化的问题，CPU 的 MMU 中有专门缓存内存中页表项的 Cache -- TLB(Translation Look-aside buffer),  在进行虚拟地址到物理地址之间转换时，无需去内存中读取页表项。Cache 和 TLB 的读取顺序取决于 CPU 架构的实现:
+ 先读Cache后读TLB为逻辑 Cache，因为要使用进程虚拟地址去 Cache 中匹配数据；
+ 先读TLB后读Cache为物理Cache，直接使用物理地址去Cache中匹配数据。

![](https://img-blog.csdnimg.cn/img_convert/fd5a4cac1858d625de79e265f3f1aef9.png)

当数据量过大，内存申请的页数变多，更容易造成 TLB miss。

实际上，以上所说的种种问题，都是常见的针对 Linux 性能问题，dpdk 主要就是将针对以上的种种问题的优化手段综合起来：

1. UIO Driver + PMD：使用用户态Driver，将数据的处理放在用户态，可以减少内核和用户态的上下文切换；使用 Poll（轮训）的方式接收数据，避免频繁中断带来的性能影响；

2. HugePages + Share Mem: 开启大页内存可以大大减少 TLB miss 的概率；使用共享内存实现零拷贝传递数据。

3. CPU 内核绑定：设置绑定的内核，降低跨核迁移引起的性能损耗；

## 编译

### cgo 配置

主要通过两个注释关键字声明 gcc 编译参数：CFLAGS 和 LDFLAGS

https://ftp.gnu.org/old-gnu/Manuals/ld-2.9.1/html_node/ld_3.html
https://pkg.go.dev/cmd/cgo@go1.16beta1

https://blog.csdn.net/brooknew/article/details/8463822

LDFLAGS 中 -whole-archive 和 --no-whole-archive 是ld专有的命令行参数，gcc 并不认识，要通gcc传递到 ld，需要在他们前面加 -Wl，字串。

编译时使用

go env -w CGO_LDFLAGS_ALLOW='-Wl,.*'

或

CGO_LDFLAGS_ALLOW='-Wl,.*' go build


./example --conf config.ini --proc-type=primary --proc-id=0