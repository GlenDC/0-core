package builtin

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"regexp"
	"strconv"

	"github.com/zero-os/0-core/base/pm"
	"github.com/zero-os/0-core/base/pm/core"
	"github.com/zero-os/0-core/base/pm/process"
)

/*
Implementation for disk.getinfo
Note that, the rest of the disk extension implementation is done in conf/disk.toml file
*/

var (
	freeSpaceRegex = regexp.MustCompile(`(?m:^\s*(\d+)B\s+(\d+)B\s+(\d+)B\s+Free Space$)`)
	partTableRegex = regexp.MustCompile(`Partition Table: (\w+)`)
)

type diskMgr struct{}

func init() {
	d := (*diskMgr)(nil)
	pm.CmdMap["disk.getinfo"] = process.NewInternalProcessFactory(d.info)
	pm.CmdMap["disk.list"] = process.NewInternalProcessFactory(d.list)
}

type diskInfo struct {
	Disk string `json:"disk"`
	Part string `json:"part"`
}

type DiskFreeBlock struct {
	Start uint64 `json:"start"`
	End   uint64 `json:"end"`
	Size  uint64 `json:"size"`
}

type DiskInfoResult struct {
	lsblkResult
	Start     uint64          `json:"start"`
	End       uint64          `json:"end"`
	Size      uint64          `json:"size"`
	BlockSize uint64          `json:"blocksize"`
	Table     string          `json:"table"`
	Free      []DiskFreeBlock `json:"free"`
}

type lsblkListResult struct {
	BlockDevices []lsblkResult `json:"blockdevices"`
}

type lsblkResult struct {
	Name       string        `json:"name"`
	Kname      string        `json:"kname"`
	MajMin     string        `json:"maj:min"`
	Fstype     interface{}   `json:"fstype"`
	Mountpoint interface{}   `json:"mountpoint"`
	Label      interface{}   `json:"label"`
	UUID       interface{}   `json:"uuid"`
	Parttype   interface{}   `json:"parttype"`
	Partlabel  interface{}   `json:"partlabel"`
	Partuuid   interface{}   `json:"partuuid"`
	Partflags  interface{}   `json:"partflags"`
	Ra         string        `json:"ra"`
	Ro         string        `json:"ro"`
	Rm         string        `json:"rm"`
	Hotplug    string        `json:"hotplug"`
	Model      string        `json:"model"`
	Serial     string        `json:"serial"`
	Size       string        `json:"size"`
	State      interface{}   `json:"state"`
	Owner      string        `json:"owner"`
	Group      string        `json:"group"`
	Mode       string        `json:"mode"`
	Alignment  string        `json:"alignment"`
	MinIo      string        `json:"min-io"`
	OptIo      string        `json:"opt-io"`
	PhySec     string        `json:"phy-sec"`
	LogSec     string        `json:"log-sec"`
	Rota       string        `json:"rota"`
	Sched      interface{}   `json:"sched"`
	RqSize     string        `json:"rq-size"`
	Type       string        `json:"type"`
	DiscAln    string        `json:"disc-aln"`
	DiscGran   string        `json:"disc-gran"`
	DiscMax    string        `json:"disc-max"`
	DiscZero   string        `json:"disc-zero"`
	Wsame      string        `json:"wsame"`
	Wwn        interface{}   `json:"wwn"`
	Rand       string        `json:"rand"`
	Pkname     interface{}   `json:"pkname"`
	Hctl       interface{}   `json:"hctl"`
	Tran       string        `json:"tran"`
	Subsystems string        `json:"subsystems"`
	Rev        interface{}   `json:"rev"`
	Vendor     interface{}   `json:"vendor"`
	Children   []lsblkResult `json:"children,omitempty"`
}

func (d *diskMgr) readUInt64(p string) (uint64, error) {
	bytes, err := ioutil.ReadFile(p)
	if err != nil {
		return 0, err
	}
	var r uint64
	if _, err := fmt.Sscanf(string(bytes), "%d", &r); err != nil {
		return 0, err
	}

	return r, nil
}

func (d *diskMgr) lsblk(dev string) (*lsblkResult, error) {
	result, err := pm.GetManager().System("lsblk", "-O", "-J", fmt.Sprintf("/dev/%s", dev))

	if err != nil {
		return nil, err
	}

	cmdOutput := struct {
		BlockDevices []lsblkResult `json:"blockdevices"`
	}{}
	if err := json.Unmarshal([]byte(result.Streams.Stdout()), &cmdOutput); err != nil {
		return nil, err
	}

	if len(cmdOutput.BlockDevices) >= 1 {
		return &cmdOutput.BlockDevices[0], nil

	}

	return nil, fmt.Errorf("not device with the name /dev/%s", dev)

}
func (d *diskMgr) blockSize(dev string) (uint64, error) {
	return d.readUInt64(fmt.Sprintf("/sys/block/%s/queue/logical_block_size", dev))
}

func (d *diskMgr) getTableInfo(disk string) (string, []DiskFreeBlock, error) {
	blocks := make([]DiskFreeBlock, 0)
	result, err := pm.GetManager().System("parted", fmt.Sprintf("/dev/%s", disk), "unit", "B", "print", "free")

	if err != nil {
		return "", blocks, err
	}

	table := ""
	tableMatch := partTableRegex.FindStringSubmatch(result.Streams.Stdout())
	if len(tableMatch) == 2 {
		table = tableMatch[1]
	}

	matches := freeSpaceRegex.FindAllStringSubmatch(result.Streams.Stdout(), -1)
	for _, match := range matches {
		bstart, _ := strconv.ParseUint(match[1], 10, 64)
		bend, _ := strconv.ParseUint(match[2], 10, 64)
		bsize, _ := strconv.ParseUint(match[3], 10, 64)

		blocks = append(blocks, DiskFreeBlock{
			Start: bstart,
			End:   bend,
			Size:  bsize,
		})
	}

	return table, blocks, nil
}

func (d *diskMgr) diskInfo(disk string) (*DiskInfoResult, error) {

	var info DiskInfoResult

	lsblk, err := d.lsblk(disk)
	if err != nil {
		return nil, err
	}
	info.lsblkResult = *lsblk

	bs, err := d.blockSize(disk)
	if err != nil {
		return nil, err
	}

	size, err := d.readUInt64(fmt.Sprintf("/sys/block/%s/size", disk))
	if err != nil {
		return nil, err
	}
	info.Size = size * bs
	info.End = (size * bs) - 1

	info.BlockSize = bs
	//get free blocks.
	table, blocks, err := d.getTableInfo(disk)
	if err != nil {
		return nil, err
	}
	info.Table = table
	info.Free = blocks

	return &info, nil
}

func (d *diskMgr) partInfo(disk, part string) (*DiskInfoResult, error) {
	var info DiskInfoResult

	lsblk, err := d.lsblk(part)
	if err != nil {
		return nil, err
	}
	info.lsblkResult = *lsblk

	bs, err := d.blockSize(disk)
	if err != nil {
		return nil, err
	}

	start, err := d.readUInt64(fmt.Sprintf("/sys/block/%s/%s/start", disk, part))
	if err != nil {
		return nil, err
	}

	size, err := d.readUInt64(fmt.Sprintf("/sys/block/%s/%s/size", disk, part))
	if err != nil {
		return nil, err
	}

	info.Start = start * bs
	info.Size = size * bs
	info.End = info.Start + info.Size - 1

	info.BlockSize = bs
	info.Free = make([]DiskFreeBlock, 0) //this is just to make the return consistent
	return &info, nil
}

func (d *diskMgr) info(cmd *core.Command) (interface{}, error) {
	var args diskInfo

	if err := json.Unmarshal(*cmd.Arguments, &args); err != nil {
		return nil, err
	}

	if args.Part == "" {
		return d.diskInfo(args.Disk)
	}

	return d.partInfo(args.Disk, args.Part)
}

func (d *diskMgr) list(cmd *core.Command) (interface{}, error) {
	result, err := pm.GetManager().System("lsblk", "--json", "--output-all", "--bytes", "--exclude", "1,2")
	if err != nil {
		return nil, err
	}

	var disks lsblkListResult
	if err := json.Unmarshal([]byte(result.Streams.Stdout()), &disks); err != nil {
		return nil, err
	}
	ret := []lsblkResult{}
	for _, disk := range disks.BlockDevices {
		if disk.Type == "disk" {
			ret = append(ret, disk)
		}
	}
	disks.BlockDevices = ret
	return disks, nil
}
