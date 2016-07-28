package view

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"unsafe"
)

var errBadLine = errors.New("illegal line format")
var errBadSubnet = errors.New("illegal subnet format")
var errDupNet = errors.New("duplication subnet not allowed")
var errBadAddress = errors.New("illegal address format")

var bigEndian bool

const (
	viewLineDelimite = " "
	viewLineFields   = 5
	viewLineNetField = 4

	viewNetFieldPrefix = "{"
	viewNetFieldSuffix = ";};};"
	viewNetFieldSep    = ";"

	viewSubnetDelimite = "/"

	viewIpaddrDelimite = "."
)

type ViewInfo struct {
	area   string
	subnet string
}

type viewNode struct {
	left  *viewNode
	right *viewNode
	vinfo *ViewInfo
}

type View struct {
	root     viewNode
	fileName string
	lineNum  uint
}

func init() {
	var i int32 = 0x01020304

	u := unsafe.Pointer(&i)
	pb := (*byte)(u)

	b := *pb
	if b == 0x01 {
		bigEndian = true
	}
}

func viewUintAddr(netaddr [4]uint8) (unetaddr uint32) {
	buf := bytes.NewReader(netaddr[:])

	binary.Read(buf, binary.BigEndian, &unetaddr)

	return
}

func New() *View {
	return &View{}
}

func (v *View) viewInsert(unetaddr uint32, mask int, info *ViewInfo) (err error) {
	var bit uint32 = 0x80000000

	node := &v.root
	next := &v.root

	for mask > 0 {
		if unetaddr&bit != 0 {
			next = node.right
		} else {
			next = node.left
		}

		if next == nil {
			break
		}

		bit >>= 1

		node = next

		mask--
	}

	if next != nil {
		if node.vinfo != nil {
			return errDupNet
		}

		node.vinfo = info
		return nil
	}

	for mask > 0 {
		next = &viewNode{}

		if unetaddr&bit != 0 {
			node.right = next
		} else {
			node.left = next
		}

		mask--
		bit >>= 1
		node = next
	}

	node.vinfo = info

	return nil
}

func (v *View) viewSubnet(subnet []byte, area []byte) (err error) {
	var (
		netaddr [4]uint8
		mask    int
	)

	netfs := bytes.Split(subnet, []byte(viewSubnetDelimite))
	if len(netfs) != 2 {
		return errBadSubnet
	}

	ipfs := bytes.Split(netfs[0], []byte(viewIpaddrDelimite))
	if len(ipfs) != 4 {
		return errBadSubnet
	}

	for i := 0; i < 4; i++ {
		baddr, err := strconv.Atoi(string(ipfs[i]))
		if err != nil {
			return errBadSubnet
		}
		netaddr[i] = uint8(baddr)
	}

	mask, err = strconv.Atoi(string(netfs[1]))
	if err != nil {
		return errBadSubnet
	}

	unetaddr := viewUintAddr(netaddr)

	v.viewInsert(unetaddr, mask, &ViewInfo{area: string(area), subnet: string(subnet)})

	return nil
}

func (v *View) viewLine(line []byte) (err error) {
	fields := bytes.Fields(line)
	if len(fields) != viewLineFields {
		return fmt.Errorf("%s Line:%d\n", errBadLine.Error(), v.lineNum)
	}

	subNetBuf := fields[viewLineNetField]
	if bytes.HasPrefix(fields[viewLineNetField], []byte(viewNetFieldPrefix)) {
		subNetBuf = subNetBuf[1:]
	}

	if bytes.HasSuffix(fields[viewLineNetField], []byte(viewNetFieldSuffix)) {
		subNetBuf = subNetBuf[:len(subNetBuf)-len(viewNetFieldSuffix)]
	}

	subNets := bytes.Split(subNetBuf, []byte(viewNetFieldSep))
	if len(subNets) < 1 {
		return
	}

	for i := 0; i < len(subNets); i++ {
		if err = v.viewSubnet(subNets[i], fields[1]); err != nil {
			return fmt.Errorf("%s Line:%d\n", err.Error(), v.lineNum)
		}
	}

	return nil
}

func (v *View) Init(fileName string) (err error) {
	file, err := os.Open(fileName)
	if err != nil {
		return err
	}

	defer file.Close()

	v.fileName = fileName

	reader := bufio.NewReader(file)

	var line []byte
	for err == nil {
		line, err = reader.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return err
		}

		line = bytes.TrimSpace(line)

		v.lineNum++

		if len(line) == 0 {
			if err == io.EOF {
				break
			}
			continue
		}

		if lerr := v.viewLine(line); lerr != nil {
			return lerr
		}

		if err == io.EOF {
			break
		}
	}

	return nil
}

func (v *View) viewLookup(unetaddr uint32) (info *ViewInfo) {
	var (
		bit  uint32    = 0x80000000
		node *viewNode = &v.root
	)

	for node != nil {
		if node.vinfo != nil {
			info = node.vinfo
		}

		if unetaddr&bit != 0 {
			node = node.right
		} else {
			node = node.left
		}

		bit >>= 1
	}

	return
}

func (v *View) Lookup(addr string) (info *ViewInfo, err error) {
	baddr := []byte(addr)

	ipfs := bytes.Split(baddr, []byte(viewIpaddrDelimite))
	if len(ipfs) != 4 {
		return nil, errBadAddress
	}

	var netaddr [4]uint8

	for i := 0; i < 4; i++ {
		baddr, err := strconv.Atoi(string(ipfs[i]))
		if err != nil {
			return nil, errBadAddress
		}
		netaddr[i] = uint8(baddr)
	}

	unetaddr := viewUintAddr(netaddr)

	return v.viewLookup(unetaddr), nil
}
