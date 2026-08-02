package dashboard

import "time"

type Snapshot struct {
	Status        string      `json:"status"`
	Message       string      `json:"message,omitempty"`
	Host          Host        `json:"host"`
	Environment   Environment `json:"environment"`
	Projects      []Project   `json:"projects"`
	SSHResponseMS int64       `json:"sshResponseMs,omitempty"`
	UpdatedAt     time.Time   `json:"updatedAt"`
}

type Host struct {
	Name   string `json:"name"`
	OS     string `json:"os,omitempty"`
	CPU    CPU    `json:"cpu"`
	Memory uint64 `json:"memoryBytes,omitempty"`
	GPU    GPU    `json:"gpu"`
	Disks  []Disk `json:"disks"`
}

type CPU struct {
	Model             string `json:"model,omitempty"`
	PhysicalCores     int    `json:"physicalCores,omitempty"`
	LogicalProcessors int    `json:"logicalProcessors,omitempty"`
}

type GPU struct {
	Model  string `json:"model,omitempty"`
	Memory uint64 `json:"memoryBytes,omitempty"`
}

type Disk struct {
	Name      string `json:"name"`
	Total     uint64 `json:"totalBytes"`
	Available uint64 `json:"availableBytes"`
}

type Environment struct {
	Distribution    string `json:"distribution,omitempty"`
	Kernel          string `json:"kernel,omitempty"`
	Processors      int    `json:"processors,omitempty"`
	Memory          uint64 `json:"memoryBytes,omitempty"`
	MemoryAvailable uint64 `json:"memoryAvailableBytes,omitempty"`
	Disk            Disk   `json:"disk"`
}

type Project struct {
	Name         string   `json:"name"`
	Path         string   `json:"path"`
	Branch       string   `json:"branch,omitempty"`
	Technologies []string `json:"technologies"`
}

type Collector interface {
	Collect() Snapshot
}
