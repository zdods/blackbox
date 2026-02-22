package pkg

// Message types for agent-bastion WebSocket protocol.
const (
	TypeAuth      = "auth"
	TypeAuthOK    = "auth_ok"
	TypeAuthError = "auth_error"
	TypeListDir   = "list_dir"
	TypeReadFile  = "read_file"
	TypeWriteFile = "write_file"
	TypeGetMeta    = "get_meta"
	TypeDeleteFile = "delete_file"
	TypeGetDisk    = "get_disk"
)

// Auth is sent by agent to bastion after WebSocket connect.
type Auth struct {
	Type  string `json:"type"` // "auth"
	Token string `json:"token"`
}

// AuthOK is sent by bastion to agent after successful auth.
type AuthOK struct {
	Type    string `json:"type"` // "auth_ok"
	AgentID string `json:"agent_id"`
}

// AuthError is sent by bastion when agent auth fails.
type AuthError struct {
	Type  string `json:"type"` // "auth_error"
	Error string `json:"error"`
}

// ListDirRequest is sent by bastion to agent (path relative to hosted root).
type ListDirRequest struct {
	Type      string `json:"type"` // "list_dir"
	RequestID string `json:"request_id"`
	Path      string `json:"path"`
}

// FileEntry is one entry in a directory listing.
type FileEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
	Mtime string `json:"mtime"` // RFC3339
}

// ListDirResponse is sent by agent to bastion.
type ListDirResponse struct {
	Type      string      `json:"type"` // "list_dir"
	RequestID string      `json:"request_id"`
	Entries  []FileEntry  `json:"entries,omitempty"`
	Error    string       `json:"error,omitempty"`
}

// ReadFileRequest is sent by bastion to agent.
type ReadFileRequest struct {
	Type      string `json:"type"` // "read_file"
	RequestID string `json:"request_id"`
	Path      string `json:"path"`
	Offset    int64  `json:"offset,omitempty"`
	Size      int64  `json:"size,omitempty"` // 0 = read all
}

// ReadFileResponse is sent by agent to bastion. Data is base64-encoded.
type ReadFileResponse struct {
	Type      string `json:"type"` // "read_file"
	RequestID string `json:"request_id"`
	Data     string `json:"data,omitempty"` // base64
	Error    string `json:"error,omitempty"`
}

// WriteFileRequest is sent by bastion to agent. Data is base64-encoded.
type WriteFileRequest struct {
	Type      string `json:"type"` // "write_file"
	RequestID string `json:"request_id"`
	Path      string `json:"path"`
	Data     string `json:"data"` // base64
}

// WriteFileResponse is sent by agent to bastion.
type WriteFileResponse struct {
	Type      string `json:"type"` // "write_file"
	RequestID string `json:"request_id"`
	Error    string `json:"error,omitempty"`
}

// GetMetaRequest is sent by bastion to agent.
type GetMetaRequest struct {
	Type      string `json:"type"` // "get_meta"
	RequestID string `json:"request_id"`
	Path      string `json:"path"`
}

// GetMetaResponse is sent by agent to bastion.
type GetMetaResponse struct {
	Type      string `json:"type"` // "get_meta"
	RequestID string `json:"request_id"`
	Size     int64  `json:"size,omitempty"`
	Mtime    string `json:"mtime,omitempty"` // RFC3339
	IsDir    bool   `json:"is_dir,omitempty"`
	Error    string `json:"error,omitempty"`
}

// DeleteFileRequest is sent by bastion to agent.
type DeleteFileRequest struct {
	Type      string `json:"type"` // "delete_file"
	RequestID string `json:"request_id"`
	Path      string `json:"path"`
}

// DeleteFileResponse is sent by agent to bastion.
type DeleteFileResponse struct {
	Type      string `json:"type"` // "delete_file"
	RequestID string `json:"request_id"`
	Error    string `json:"error,omitempty"`
}

// GetDiskRequest is sent by bastion to agent (disk stats for hosted root volume).
type GetDiskRequest struct {
	Type      string `json:"type"` // "get_disk"
	RequestID string `json:"request_id"`
}

// GetDiskResponse is sent by agent to bastion.
type GetDiskResponse struct {
	Type       string `json:"type"` // "get_disk"
	RequestID  string `json:"request_id"`
	FreeBytes  int64  `json:"free_bytes,omitempty"`
	TotalBytes int64  `json:"total_bytes,omitempty"`
	Error      string `json:"error,omitempty"`
}
