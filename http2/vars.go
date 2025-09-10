package http2

/*
HTTP/2 protocol as defined in RFC 7540.

The basic flow is:
- Client connects and sends a connection preface (see ClientPreface)
- Server responds with a SETTINGS frame
- Communication proceeds with frames (see frame types below)

Each frame has a 9-byte header:
- Length (24 bits): Length of the frame payload
- Type (8 bits): Frame type (e.g., DATA, SETTINGS, PING, etc.)
- Flags (8 bits): Frame flags (e.g., ACK, END_STREAM, etc.)
- R (1 bit): Reserved bit
- Stream Identifier (31 bits): Identifies the stream the frame belongs to
*/

// Frame types
const (
	DataFrameType         byte = 0x0
	HeadersFrameType      byte = 0x1
	SettingsFrameType     byte = 0x4
	WindowUpdateFrameType byte = 0x7
	PingFrameType         byte = 0x8
	ContinuationFrameType byte = 0x9
)

// Flags for SETTINGS frames
const AckFlag byte = 0x1

// Flags for HEADERS frames
const (
	EndStreamFlag  byte = 0x1
	EndHeadersFlag byte = 0x4
	PaddedFlag     byte = 0x8
	PriorityFlag   byte = 0x20
)

// IndexedHeaderMask - Indexed Header Field representation (1xxxxxxx)
const IndexedHeaderMask byte = 0x80

// LiteralIncIndexMask - Literal Header Field with incremental indexing (01xxxxxx)
const LiteralIncIndexMask byte = 0x40

// LiteralNoIndexMask - Literal Header Field without indexing (0000xxxx)
const LiteralNoIndexMask byte = 0x00

// LiteralNeverIndexMask - Literal Header Field never indexed (0001xxxx)
const LiteralNeverIndexMask byte = 0x10

const (
	ClientPreface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"
)
