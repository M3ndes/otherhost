package dashboard

import "time"

type Snapshot struct {
	Mode          string      `json:"mode"`
	Status        string      `json:"status"`
	Message       string      `json:"message,omitempty"`
	Host          Host        `json:"host"`
	Environment   Environment `json:"environment"`
	Projects      []Project   `json:"projects"`
	SSHResponseMS int64       `json:"sshResponseMs,omitempty"`
	UpdatedAt     time.Time   `json:"updatedAt"`
	Connection    Connection  `json:"connection"`
	Setup         HostSetup   `json:"setup"`
	Clients       []Client    `json:"clients"`
	Sessions      []Session   `json:"sessions"`
}

type Connection struct {
	State          string `json:"state"`
	Paired         bool   `json:"paired"`
	HostName       string `json:"hostName,omitempty"`
	HostAddress    string `json:"hostAddress,omitempty"`
	SSHUser        string `json:"sshUser,omitempty"`
	SSHPort        int    `json:"sshPort,omitempty"`
	IdentityPinned bool   `json:"identityPinned"`
	Message        string `json:"message,omitempty"`
}

type HostSetup struct {
	State   string      `json:"state"`
	Message string      `json:"message,omitempty"`
	Steps   []SetupStep `json:"steps"`
	Busy    bool        `json:"busy"`
}

type SetupStep struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type Client struct {
	Fingerprint string    `json:"fingerprint"`
	Name        string    `json:"name,omitempty"`
	Authorized  bool      `json:"authorized"`
	AddedAt     time.Time `json:"addedAt,omitempty"`
}

type Session struct {
	Address string `json:"address"`
	State   string `json:"state"`
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
