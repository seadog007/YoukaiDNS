package dns

// DNS record types
const (
	TypeA   uint16 = 1  // IPv4 address
	TypeTXT uint16 = 16 // Text string
)

// DNS response codes
const (
	RcodeNoError  = 0 // No error
	RcodeNXDomain = 3 // Name does not exist
)

// DNS message flags
const (
	FlagQR     = 0x8000 // Query/Response
	FlagAA     = 0x0400 // Authoritative Answer
	FlagRD     = 0x0100 // Recursion Desired
	FlagRA     = 0x0080 // Recursion Available
	FlagResponse = FlagQR | FlagAA
)

// MessageHeader represents a DNS message header
type MessageHeader struct {
	ID      uint16 // Query identifier
	Flags   uint16 // Message flags
	QdCount uint16 // Number of questions
	AnCount uint16 // Number of answers
	NsCount uint16 // Number of authority records
	ArCount uint16 // Number of additional records
}

// Question represents a DNS question
type Question struct {
	Name  string // Domain name
	Type  uint16 // Record type
	Class uint16 // Class (usually 1 for IN)
}

// ResourceRecord represents a DNS resource record
type ResourceRecord struct {
	Name     string // Domain name
	Type     uint16 // Record type
	Class    uint16 // Class (usually 1 for IN)
	TTL      uint32 // Time to live
	Data     []byte // Record data
	DataLen  uint16 // Length of data
}

// Message represents a complete DNS message
type Message struct {
	Header      MessageHeader
	Questions   []Question
	Answers     []ResourceRecord
	Authorities []ResourceRecord
	Additionals []ResourceRecord
}

