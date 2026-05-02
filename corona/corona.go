package corona

const (
	Version          uint16 = 2
	DefaultChunkSize        = 8 << 20
)

type PackOptions struct {
	ImagePath    string
	ArtifactPath string
	ChunkSize    int64
	Workers      int
	Progress     func(Progress)
}

type WriteOptions struct {
	ArtifactPath string
	TargetPath   string
	Workers      int
	WriteOrder   WriteOrder
	Progress     func(Progress)
}

type WriteImageOptions struct {
	ImagePath  string
	TargetPath string
	ChunkSize  int64
	Workers    int
	WriteOrder WriteOrder
	Progress   func(Progress)
}

type Progress struct {
	ProcessedBytes uint64
	TotalBytes     uint64
	Percent        int
}

type WriteOrder string

const (
	WriteOrderSequential WriteOrder = "sequential"
	WriteOrderStriped    WriteOrder = "striped"
)

type Info struct {
	Version         uint16
	ImageSize       uint64
	ChunkSize       uint64
	UsefulBytes     uint64
	StoredBytes     uint64
	OperationNum    int
	FSType          uint8
	FSVersion       uint16
	FSBlockSize     uint64
	AllocatedSHA256 string
}
