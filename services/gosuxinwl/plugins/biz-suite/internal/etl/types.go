// Package etl imports an explicit, versioned ruoyi export without changing host
// accounts. Dry runs perform database reads only, including message authorization.
package etl

type Identity struct {
	WeComUserID     string `json:"wecom_userid,omitempty"`
	EmployeeNo      string `json:"employee_no,omitempty"`
	Mobile          string `json:"mobile,omitempty"`
	Username        string `json:"username,omitempty"`
	ConfirmUsername bool   `json:"confirm_username,omitempty"`
	HostUserID      int64  `json:"host_user_id,omitempty"`
}
type Employee struct {
	SourceID string `json:"source_id"`
	Identity
	Name       string `json:"name"`
	Email      string `json:"email,omitempty"`
	Position   string `json:"position,omitempty"`
	Status     string `json:"status,omitempty"`
	HostDeptID int64  `json:"host_dept_id,omitempty"`
	EntryDate  string `json:"entry_date,omitempty"`
	QuitDate   string `json:"quit_date,omitempty"`
}
type Clock struct {
	SourceID         string `json:"source_id"`
	EmployeeSourceID string `json:"employee_source_id"`
	ClockTime        string `json:"clock_time"`
	ClockType        string `json:"clock_type"`
	Status           string `json:"status,omitempty"`
	Latitude         string `json:"latitude,omitempty"`
	Longitude        string `json:"longitude,omitempty"`
	Address          string `json:"address,omitempty"`
}
type Absence struct {
	SourceID         string  `json:"source_id"`
	EmployeeSourceID string  `json:"employee_source_id"`
	ApprovalStatus   string  `json:"approval_status"`
	StartTime        string  `json:"start_time"`
	EndTime          string  `json:"end_time"`
	Duration         float64 `json:"duration"`
	Reason           string  `json:"reason,omitempty"`
	LeaveType        string  `json:"leave_type,omitempty"`
	Destination      string  `json:"destination,omitempty"`
}
type IMUser struct {
	SourceID string `json:"source_id"`
	Identity
}
type Message struct {
	SourceID         string `json:"source_id"`
	SenderSourceID   string `json:"sender_source_id"`
	ReceiverSourceID string `json:"receiver_source_id,omitempty"`
	ConversationType string `json:"conversation_type"`
	ConversationID   string `json:"conversation_id,omitempty"`
	MessageType      string `json:"message_type"`
	Content          string `json:"content"`
	SendTime         string `json:"send_time"`
}
type Document struct {
	Version   int        `json:"version"`
	Employees []Employee `json:"employees,omitempty"`
	Clocks    []Clock    `json:"clocks,omitempty"`
	Leaves    []Absence  `json:"leaves,omitempty"`
	Outs      []Absence  `json:"outs,omitempty"`
	IMUsers   []IMUser   `json:"im_users,omitempty"`
	Messages  []Message  `json:"messages,omitempty"`
}
type Result struct {
	Entity   string `json:"entity"`
	SourceID string `json:"source_id"`
	TargetID string `json:"target_id,omitempty"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}
type Report struct {
	DryRun  bool     `json:"dry_run"`
	Ready   int      `json:"ready"`
	Applied int      `json:"applied"`
	Failed  int      `json:"failed"`
	Results []Result `json:"results"`
}
