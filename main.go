package main

import (
	"fmt"
	"github.com/tarm/serial"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	CODE_LENGTH = 4
	FIXED_SUM   = 3
)

func CheckBufCmdCode(buf []byte, n int) (succ bool) {
	sum := 0
	for i := 0; i < n-1; i++ {
		sum += int(buf[i] - '0')
	}

	if sum+FIXED_SUM == int(buf[n-1]-'0') {
		fmt.Println("输入 cmd 成功")
		return true
	}

	fmt.Println("输入的 cmd 无效")
	return false
}

func CheckStrCmdCode(buf string, n int) (succ bool) {
	sum := 0
	for i := 0; i < n-1; i++ {
		sum += int(buf[i] - '0')
	}
	if buf[n-1] >= 'A' && buf[n-1] <= 'F' {
		if sum == (10 + int(buf[n-1]-'A')) {
			return true
		}
	} else if buf[n-1] >= 'a' && buf[n-1] <= 'f' {
		if sum == (10 + int(buf[n-1]-'a')) {
			return true
		}
	} else {
		if sum == int(buf[n-1]-'0') {
			return true
		}
	}

	fmt.Println("输入的 cmd 有误: ", buf)
	return false
}

func CompreBufEqual(preBuf, nextBuf []byte, n int) (equal bool) {
	for i := 0; i < n; i++ {
		if preBuf[i] != nextBuf[i] {
			return false
		}
	}

	return true
}

func main() {
	uartDev := ""
	fmt.Println("请输入串口设备号: ")
	fmt.Scanf("%s", &uartDev)

	c := &serial.Config{Name: uartDev, Baud: 115200}
	uart, err := serial.OpenPort(c)
	if err != nil {
		log.Fatal("读取串口设备出错, Err: ", err)
	}

	csiAddr := ""
	fmt.Println("请输入 CSI 文件位置: ")
	fmt.Scanf("%s", &csiAddr)

	fdCSI, err := os.OpenFile(csiAddr, os.O_APPEND|os.O_RDWR|os.O_CREATE, os.ModePerm)
	if err != nil {
		panic("读取 CSI 文件出错")
	}

	fdRecord, err := os.OpenFile("output.log", os.O_APPEND|os.O_RDWR|os.O_CREATE, os.ModePerm)
	if err != nil {
		panic("读取 output 文件出错")
	}

	preBuf := make([]byte, 10)
	nextBuf := make([]byte, 10)
	ackBuf := make([]byte, 10)

	var (
		cmd string

		recvCmd bool

		sendCmd bool = false
		waitAck bool = false
		recvAck bool = false
	)
	wg := sync.WaitGroup{}

	// received cmd
	go func() {
		wg.Add(1)
		for {
			fmt.Printf("请输入 cmd 指令: ")
			fmt.Scanf("%s\n", &cmd)
			succ := CheckStrCmdCode(cmd, CODE_LENGTH)
			if succ != false {
				preBuf = ([]byte(cmd))[0:CODE_LENGTH]
				equal := CompreBufEqual(preBuf, nextBuf, CODE_LENGTH)
				if equal != false {
					fmt.Println("输入 cmd 与上次相同")
				} else {
					fmt.Println("输入 cmd 有效: ", cmd)
					t := time.Now()
					tUnix := strconv.Itoa(int(t.UnixNano()))
					nt := t.Format("2006-01-02 15:04:05")
					_, err := fdCSI.Write([]byte(nt + "===" + tUnix + "===" + cmd + "===" + "ready\r\n"))
					if err != nil {
						fmt.Println("写入 CSI 文件出错, Err: ", err)
					}

					_, err = fdRecord.Write([]byte(nt + "===" + tUnix + "===" + cmd + "===" + "ready\r\n"))
					if err != nil {
						fmt.Println("写入 CSI 文件出错, Err: ", err)
					}

					nextBuf = preBuf
					recvCmd = true
					recvAck = false
				}
			}
			time.Sleep(50 * time.Millisecond)
		}

		wg.Done()
	}()

	// send cmd to esp8266 by uart
	go func() {
		wg.Add(1)
		for {
			if recvCmd == true && waitAck == false {
				n, err := uart.Write(preBuf[:CODE_LENGTH])
				if err != nil {
					fmt.Println("串口发送数据失败, Err: ", err)
					fmt.Println("重试……")
					continue
				}

				if n != CODE_LENGTH {
					fmt.Println("串口发送数据不完全: ", preBuf[:n])
					fmt.Println("重试……")
					continue
				}

				sendCmd = true
				waitAck = true
			}
			time.Sleep(50 * time.Millisecond)
		}

		wg.Done()
	}()

	// recv ack from esp8266 by uart
	go func() {
		wg.Add(1)
		for {
			if sendCmd == true && waitAck == true {
				n, err := uart.Read(ackBuf)
				if err != nil {
					fmt.Println("串口接收数据错误, Err: ", err)
				}

				if strings.Index(string(ackBuf[:n]), "succ") != -1 {
					t := time.Now()
					tUnix := strconv.Itoa(int(t.UnixNano()))
					nt := t.Format("2006-01-02 15:04:05")
					_, err := fdCSI.Write([]byte(nt + "===" + tUnix + "===" + cmd + "===" + "work\r\n"))
					if err != nil {
						fmt.Println("写入 CSI 文件出错, Err: ", err)
					}

					_, err = fdRecord.Write([]byte(nt + "===" + tUnix + "===" + cmd + "===" + "work\r\n"))
					if err != nil {
						fmt.Println("写入 CSI 文件出错, Err: ", err)
					}
					fmt.Println("新的 cmd 设置成功")
				} else if strings.Index(string(ackBuf[:n]), "fail") != -1 {
					fmt.Println("新的 cmd 设置失败")
					t := time.Now()
					tUnix := strconv.Itoa(int(t.UnixNano()))
					nt := t.Format("2006-01-02 15:04:05")
					_, err := fdCSI.Write([]byte(nt + "===" + tUnix + "===" + "===" + cmd + "failed\r\n"))
					if err != nil {
						fmt.Println("写入 CSI 文件出错, Err: ", err)
					}

					_, err = fdRecord.Write([]byte(nt + "===" + tUnix + "===" + "===" + cmd + "failed\r\n"))
					if err != nil {
						fmt.Println("写入 CSI 文件出错, Err: ", err)
					}
				} else {
					fmt.Println("串口接收")
					fmt.Println(string(ackBuf[:]))
				}
				ackBuf = make([]byte, 10)

				waitAck = false
				sendCmd = false
				recvCmd = false
			}

			time.Sleep(50 * time.Millisecond)
		}
		wg.Done()
	}()

	time.Sleep(1 * time.Second)
	wg.Wait()

}
