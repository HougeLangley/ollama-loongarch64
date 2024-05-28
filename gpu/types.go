package gpu

import (
	"fmt"
	"log/slog"

	"github.com/ollama/ollama/format"
)

type memInfo struct {
	TotalMemory uint64 `json:"total_memory,omitempty"`
	FreeMemory  uint64 `json:"free_memory,omitempty"`
}

// Beginning of an `ollama info` command
type GpuInfo struct {
	memInfo
	Library string `json:"library,omitempty"`

	// Optional variant to select (e.g. versions, cpu feature flags)
	Variant string `json:"variant,omitempty"`

	// MinimumMemory represents the minimum memory required to use the GPU
	MinimumMemory uint64 `json:"-"`

	// Any extra PATH/LD_LIBRARY_PATH dependencies required for the Library to operate properly
	DependencyPath string `json:"lib_path,omitempty"`

	// GPU information
	ID      string `json:"gpu_id"`  // string to use for selection of this specific GPU
	Name    string `json:"name"`    // user friendly name if available
	Compute string `json:"compute"` // Compute Capability or gfx

	// Driver Information - TODO no need to put this on each GPU
	DriverMajor int `json:"driver_major,omitempty"`
	DriverMinor int `json:"driver_minor,omitempty"`

	// TODO other performance capability info to help in scheduling decisions
}

type GpuInfoList []GpuInfo

// Split up the set of gpu info's by Library and variant
func (l GpuInfoList) ByLibrary() []GpuInfoList {
	resp := []GpuInfoList{}
	libs := []string{}
	for _, info := range l {
		found := false
		requested := info.Library
		if info.Variant != "" {
			requested += "_" + info.Variant
		}
		for i, lib := range libs {
			if lib == requested {
				resp[i] = append(resp[i], info)
				found = true
				break
			}
		}
		if !found {
			libs = append(libs, info.Library)
			resp = append(resp, []GpuInfo{info})
		}
	}
	return resp
}

// Report the GPU information into the log an Info level
func (l GpuInfoList) LogDetails() {
	for _, g := range l {
		slog.Info("inference compute",
			"id", g.ID,
			"library", g.Library,
			"compute", g.Compute,
			"driver", fmt.Sprintf("%d.%d", g.DriverMajor, g.DriverMinor),
			"name", g.Name,
			"total", format.HumanBytes2(g.TotalMemory),
			"available", format.HumanBytes2(g.FreeMemory),
		)
	}
}

// Sort by Free Space
type ByFreeMemory []GpuInfo

func (a ByFreeMemory) Len() int           { return len(a) }
func (a ByFreeMemory) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByFreeMemory) Less(i, j int) bool { return a[i].FreeMemory < a[j].FreeMemory }
