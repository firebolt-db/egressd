//go:build linux

package conntrack

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/castai/egressd/metrics"
	"github.com/cilium/cilium/pkg/bpf"
	"github.com/cilium/cilium/pkg/defaults"
	"github.com/cilium/cilium/pkg/maps/ctmap"
	"inet.af/netaddr"
)

// https://raw.githubusercontent.com/cilium/cilium/main/pkg/defaults/defaults.go
const (
	bpfMapRoot = defaults.BPFFSRoot
	bpfMaps    = defaults.TCGlobalsPath
)

func bpfMapsExist() bool {
	path := filepath.Join(bpfMapRoot, bpfMaps)
	file, err := os.Stat(path)
	return err == nil && file != nil
}

func listRecords(maps []interface{}, clockSource ClockSource, filter EntriesFilter) ([]*Entry, error) {
	entries := make([]*Entry, 0)

	now := time.Now().UTC()

	timeDiff, err := kernelTimeDiffSecondsFunc(clockSource)
	if err != nil {
		return nil, fmt.Errorf("getting kernel time diff func: %w", err)
	}

	var fetchedCount int
	for _, m := range maps {
		m := m.(ctmap.CtMap)
		path, err := m.Path()
		if err == nil {
			err = m.Open()
		}
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("unable to open map %s: %w", path, err)
			}
		}

		cb := func(key bpf.MapKey, v bpf.MapValue) {
			fetchedCount++
			k := key.(ctmap.CtKey).ToHost().(*ctmap.CtKey4Global)
			if k.NextHeader == 0 {
				return
			}

			srcIP := k.DestAddr.IP() // Addresses are swapped due to cilium issue #21346.
			dstIP := k.SourceAddr.IP()
			val := v.(*ctmap.CtEntry)
			expireSeconds := timeDiff(int64(val.Lifetime))
			record := &Entry{
				Src:       netaddr.IPPortFrom(netaddr.IPv4(srcIP[0], srcIP[1], srcIP[2], srcIP[3]), k.SourcePort),
				Dst:       netaddr.IPPortFrom(netaddr.IPv4(dstIP[0], dstIP[1], dstIP[2], dstIP[3]), k.DestPort),
				TxBytes:   val.TxBytes,
				TxPackets: val.TxPackets,
				RxBytes:   val.RxBytes,
				RxPackets: val.RxPackets,
				Lifetime:  now.Add(time.Duration(expireSeconds) * time.Second),
				Proto:     uint8(k.NextHeader),
			}
			if filter(record) {
				entries = append(entries, record)
			}
		}
		if err = m.DumpWithCallback(cb); err != nil {
			m.Close()
			return nil, fmt.Errorf("error while collecting BPF map entries: %w", err)
		}

		// Explicitly close the ctmap after every iteration
		m.Close()
	}
	metrics.SetConntrackEntriesCount(float64(fetchedCount))
	return entries, nil
}

func initMaps() []interface{} {
	ctmap.InitMapInfo(2<<18, 2<<17, true, false, true)
	maps := ctmap.GlobalMaps(true, false)
	ctMaps := make([]interface{}, len(maps))
	for i, m := range maps {
		ctMaps[i] = m
	}
	return ctMaps
}
