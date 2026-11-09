package platform

import (
	"context"
	"errors"
	"os"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"mcl/internal/app"
)

// Layout of Apple's vm_statistics64 rev1, through the compressed-memory fields.
// Request the documented rev1 prefix (38 natural_t words) so newer SDK tails
// are not required and no kernel can write beyond this bounded Go buffer.
type vmStatistics64Rev1 struct {
	Free, Active, Inactive, Wired                                                          uint32
	ZeroFill, Reactivations, Pageins, Pageouts, Faults, CopyOnWrite, Lookups, Hits, Purges uint64
	Purgeable, Speculative                                                                 uint32
	Decompressions, Compressions, Swapins, Swapouts                                        uint64
	Compressor, Throttled, External, Internal                                              uint32
	TotalUncompressed                                                                      uint64
}
type memoryStatusAPI struct {
	hostSelf   func() uint32
	deallocate func(uint32, uint32) int32
	statistics func(uint32, int32, *vmStatistics64Rev1, *uint32) int32
	task       uint32
}

func NewMemoryProbe() app.MemoryProbe {
	load := sync.OnceValues(loadMemoryStatusAPI)
	return func(ctx context.Context) (app.MemoryStatus, error) {
		if err := ctx.Err(); err != nil {
			return app.MemoryStatus{}, err
		}
		api, err := load()
		if err != nil {
			return app.MemoryStatus{}, err
		}
		total, err := PhysicalMemory()
		if err != nil {
			return app.MemoryStatus{}, err
		}
		host := api.hostSelf()
		if host == 0 {
			return app.MemoryStatus{}, errors.New("无法读取系统内存统计")
		}
		defer api.deallocate(api.task, host)
		var stats vmStatistics64Rev1
		count := uint32(unsafe.Sizeof(stats) / 4)
		const rev1Count = 38
		if count != rev1Count {
			return app.MemoryStatus{}, errors.New("系统内存统计结构不受支持")
		}
		if result := api.statistics(host, 4, &stats, &count); result != 0 || count < rev1Count {
			return app.MemoryStatus{}, errors.New("无法读取系统内存统计")
		}
		// Wired + compressed + application pages; file-backed/purgeable caches
		// remain reclaimable. Speculative pages are already included in free pages.
		usedPages := uint64(stats.Active) + uint64(stats.Inactive) + uint64(stats.Wired) + uint64(stats.Compressor)
		reclaimable := uint64(stats.External) + uint64(stats.Purgeable)
		usedPages -= min(usedPages, reclaimable)
		used := min(total, usedPages*uint64(os.Getpagesize()))
		if err := ctx.Err(); err != nil {
			return app.MemoryStatus{}, err
		}
		return app.MemoryStatus{TotalBytes: total, AvailableBytes: total - used}, nil
	}
}
func loadMemoryStatusAPI() (*memoryStatusAPI, error) {
	library, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_LAZY|purego.RTLD_LOCAL)
	if err != nil {
		return nil, errors.New("无法载入系统内存接口")
	}
	loaded := false
	defer func() {
		if !loaded {
			purego.Dlclose(library)
		}
	}()
	api := new(memoryStatusAPI)
	for _, symbol := range []struct {
		name   string
		target any
	}{{"mach_host_self", &api.hostSelf}, {"mach_port_deallocate", &api.deallocate}, {"host_statistics64", &api.statistics}} {
		address, err := purego.Dlsym(library, symbol.name)
		if err != nil {
			return nil, errors.New("系统内存接口不可用")
		}
		purego.RegisterFunc(symbol.target, address)
	}
	address, err := purego.Dlsym(library, "mach_task_self_")
	if err != nil {
		return nil, errors.New("系统内存接口不可用")
	}
	api.task = **(**uint32)(unsafe.Pointer(&address))
	if api.task == 0 {
		return nil, errors.New("系统内存接口不可用")
	}
	loaded = true
	return api, nil
}
