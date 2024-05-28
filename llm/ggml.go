package llm

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

type GGML struct {
	container
	model
}

type model interface {
	KV() KV
	Tensors() Tensors
}

type KV map[string]any

func (kv KV) u64(key string) uint64 {
	switch v := kv[key].(type) {
	case uint64:
		return v
	case uint32:
		return uint64(v)
	case float64:
		return uint64(v)
	default:
		return 0
	}
}

func (kv KV) Architecture() string {
	if s, ok := kv["general.architecture"].(string); ok {
		return s
	}

	return "unknown"
}

func (kv KV) ParameterCount() uint64 {
	return kv.u64("general.parameter_count")
}

func (kv KV) FileType() fileType {
	if u64 := kv.u64("general.file_type"); u64 > 0 {
		return fileType(uint32(u64))
	}

	return fileTypeUnknown
}

func (kv KV) BlockCount() uint64 {
	return kv.u64(fmt.Sprintf("%s.block_count", kv.Architecture()))
}

func (kv KV) HeadCount() uint64 {
	return kv.u64(fmt.Sprintf("%s.attention.head_count", kv.Architecture()))
}

func (kv KV) HeadCountKV() uint64 {
	if headCountKV := kv.u64(fmt.Sprintf("%s.attention.head_count_kv", kv.Architecture())); headCountKV > 0 {
		return headCountKV
	}

	return 1
}

func (kv KV) GQA() uint64 {
	return kv.HeadCount() / kv.HeadCountKV()
}

func (kv KV) EmbeddingLength() uint64 {
	return kv.u64(fmt.Sprintf("%s.embedding_length", kv.Architecture()))
}

func (kv KV) ContextLength() uint64 {
	return kv.u64(fmt.Sprintf("%s.context_length", kv.Architecture()))
}

type Tensors []*Tensor

func (ts Tensors) Layers() map[string]Layer {
	layers := make(map[string]Layer)
	for _, t := range ts {
		parts := strings.Split(t.Name, ".")
		if parts[0] == "blk" {
			// join first and second part, e.g. blk.%d
			parts = append([]string{fmt.Sprintf("%s.%s", parts[0], parts[1])}, parts[2:]...)
		}

		if _, ok := layers[parts[0]]; !ok {
			layers[parts[0]] = make(Layer)
		}

		layers[parts[0]][strings.Join(parts[1:], ".")] = t
	}

	return layers
}

type Layer map[string]*Tensor

func (l Layer) size() (size uint64) {
	for _, t := range l {
		size += t.Size()
	}

	return size
}

type Tensor struct {
	Name   string `json:"name"`
	Kind   uint32 `json:"kind"`
	Offset uint64 `json:"-"`

	// Shape is the number of elements in each dimension
	Shape []uint64 `json:"shape"`

	io.WriterTo `json:"-"`
}

func (t Tensor) blockSize() uint64 {
	switch t.Kind {
	case 0, 1, 24, 25, 26, 27, 28, 30: // F32, F16, I8, I16, I32, I64, F64, BF16
		return 1
	case 2, 3, 4, 5, 6, 7, 8, 9, 20: // Q4_0, Q4_1, Q5_0, Q5_1, Q8_0, Q8_1, IQ4_NL
		return 32
	default: // All others
		return 256
	}
}

func (t Tensor) typeSize() uint64 {
	blockSize := t.blockSize()

	switch t.Kind {
	case 0: // FP32
		return 4
	case 1: // FP16
		return 2
	case 2: // Q4_0
		return 2 + blockSize/2
	case 3: // Q4_1
		return 2 + 2 + blockSize/2
	case 6: // Q5_0
		return 2 + 4 + blockSize/2
	case 7: // Q5_1
		return 2 + 2 + 4 + blockSize/2
	case 8: // Q8_0
		return 2 + blockSize
	case 9: // Q8_1
		return 4 + 4 + blockSize
	case 10: // Q2_K
		return blockSize/16 + blockSize/4 + 2 + 2
	case 11: // Q3_K
		return blockSize/8 + blockSize/4 + 12 + 2
	case 12: // Q4_K
		return 2 + 2 + 12 + blockSize/2
	case 13: // Q5_K
		return 2 + 2 + 12 + blockSize/8 + blockSize/2
	case 14: // Q6_K
		return blockSize/2 + blockSize/4 + blockSize/16 + 2
	case 15: // Q8_K
		return 2 + blockSize + 2*blockSize/16
	case 16: // IQ2_XXS
		return 2 + 2*blockSize/8
	case 17: // IQ2_XS
		return 2 + 2*blockSize/8 + blockSize/32
	case 18: // IQ3_XXS
		return 2 + blockSize/4 + blockSize/8
	case 19: // IQ1_S
		return 2 + blockSize/8 + blockSize/16
	case 20: // IQ4_NL
		return 2 + blockSize/2
	case 21: // IQ3_S
		return 2 + blockSize/4 + blockSize/8 + blockSize/32 + 4
	case 22: // IQ2_S
		return 2 + blockSize/4 + blockSize/16
	case 23: // IQ4_XS
		return 2 + 2 + blockSize/2 + blockSize/64
	case 24: // I8
		return 1
	case 25: // I16
		return 2
	case 26: // I32
		return 4
	case 27: // I64
		return 8
	case 28: // F64
		return 8
	case 29: // IQ1_M
		return blockSize/8 + blockSize/16 + blockSize/32
	default:
		return 0
	}
}

func (t Tensor) parameters() uint64 {
	var count uint64 = 1
	for _, n := range t.Shape {
		count *= n
	}
	return count
}

func (t Tensor) Size() uint64 {
	return t.parameters() * t.typeSize() / t.blockSize()
}

type container interface {
	Name() string
	Decode(io.ReadSeeker) (model, error)
}

const (
	// Magic constant for `ggml` files (unversioned).
	FILE_MAGIC_GGML = 0x67676d6c
	// Magic constant for `ggml` files (versioned, ggmf).
	FILE_MAGIC_GGMF = 0x67676d66
	// Magic constant for `ggml` files (versioned, ggjt).
	FILE_MAGIC_GGJT = 0x67676a74
	// Magic constant for `ggla` files (LoRA adapter).
	FILE_MAGIC_GGLA = 0x67676C61
	// Magic constant for `gguf` files (versioned, gguf)
	FILE_MAGIC_GGUF_LE = 0x46554747
	FILE_MAGIC_GGUF_BE = 0x47475546
)

var ErrUnsupportedFormat = errors.New("unsupported model format")

func DetectGGMLType(b []byte) string {
	switch binary.LittleEndian.Uint32(b[:4]) {
	case FILE_MAGIC_GGML:
		return "ggml"
	case FILE_MAGIC_GGMF:
		return "ggmf"
	case FILE_MAGIC_GGJT:
		return "ggjt"
	case FILE_MAGIC_GGLA:
		return "ggla"
	case FILE_MAGIC_GGUF_LE, FILE_MAGIC_GGUF_BE:
		return "gguf"
	default:
		return ""
	}
}

func DecodeGGML(rs io.ReadSeeker) (*GGML, int64, error) {
	var magic uint32
	if err := binary.Read(rs, binary.LittleEndian, &magic); err != nil {
		return nil, 0, err
	}

	var c container
	switch magic {
	case FILE_MAGIC_GGML, FILE_MAGIC_GGMF, FILE_MAGIC_GGJT:
		return nil, 0, ErrUnsupportedFormat
	case FILE_MAGIC_GGLA:
		c = &containerGGLA{}
	case FILE_MAGIC_GGUF_LE:
		c = &containerGGUF{ByteOrder: binary.LittleEndian}
	case FILE_MAGIC_GGUF_BE:
		c = &containerGGUF{ByteOrder: binary.BigEndian}
	default:
		return nil, 0, errors.New("invalid file magic")
	}

	model, err := c.Decode(rs)
	if errors.Is(err, io.EOF) {
		// noop
	} else if err != nil {
		return nil, 0, err
	}

	offset, err := rs.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, 0, err
	}

	// final model type
	return &GGML{
		container: c,
		model:     model,
	}, offset, nil
}

func (llm GGML) GraphSize(context, batch uint64) (partialOffload, fullOffload uint64) {
	embedding := llm.KV().EmbeddingLength()
	heads := llm.KV().HeadCount()
	headsKV := llm.KV().HeadCountKV()
	vocab := uint64(len(llm.KV()["tokenizer.ggml.tokens"].([]any)))

	layers := llm.Tensors().Layers()

	switch llm.KV().Architecture() {
	case "llama":
		fullOffload = 4 * batch * (1 + 4*embedding + context*(1+heads))

		partialOffload = 4 * batch * embedding
		partialOffload += max(
			4*batch*(1+embedding+max(context, embedding))+embedding*embedding*9/16+4*context*(batch*heads+embedding/heads*headsKV),
			4*batch*(embedding+vocab)+embedding*vocab*105/128,
		)

		if ffnGateExpsWeight, ok := layers["blk.0"]["ffn_gate_exps.weight"]; ok {
			// mixtral 8x22b
			ff := uint64(llm.KV()["llama.feed_forward_length"].(uint32))
			partialOffload = max(
				3*ffnGateExpsWeight.Size()+4*batch*(2*ff+headsKV+embedding+context+embedding/heads*headsKV),
				4*(context*batch*heads+context*embedding/heads*headsKV+batch*1024+embedding/heads*headsKV*batch),
			)
		} else if ffnGateWeight, ok := layers["blk.0"]["ffn_gate.0.weight"]; ok {
			// mixtral 8x7b
			ffnGateWeight1 := ffnGateWeight.Shape[1]
			fullOffload = 4 * batch * (2 + 3*embedding + context*(1+heads) + 2*headsKV + ffnGateWeight1)
			partialOffload = max(
				4*batch*(3+embedding/heads*headsKV+embedding+context*(1+heads)+ffnGateWeight1)+(embedding*embedding+3*embedding*headsKV*ffnGateWeight1)*9/16,
				4*batch*(1+2*embedding+context*(1+heads))+embedding*(6*context*headsKV/heads+embedding*9/16),
			)
		}
	case "gemma":
		fullOffload = 4 * batch * (embedding + vocab)
		partialOffload = 4*batch*(2*embedding+vocab+1) + embedding*vocab*105/128
	case "command-r":
		fullOffload = max(
			4*batch*(embedding+vocab),
			4*batch*(2+4*embedding+context*(1+heads)),
		)

		partialOffload = max(
			4*batch*(embedding+vocab)+embedding*vocab*105/128,
			4*batch*(1+2*embedding+context*(1+heads))+4*embedding*context+embedding*embedding*9/16,
		)
	case "qwen2":
		fullOffload = max(
			4*batch*(embedding+vocab),
			4*batch*(1+2*embedding+context+context*heads),
		)

		partialOffload = max(
			4*batch*(embedding+vocab)+embedding*vocab*105/128,
			4*(batch*(1+2*embedding+context*(1+heads))+embedding*(1+context)),
		)
	case "phi2":
		fullOffload = max(
			4*batch*(embedding+vocab),
			4*batch*(1+4*embedding+context+context*heads),
		)

		partialOffload = max(
			4*batch*(2*embedding+vocab)+embedding*vocab*105/128,
			4*batch*(2+3*embedding+context+context*heads),
		)
	case "stablelm":
		fullOffload = 4 * batch * (context*(1+heads) + 3*embedding + 2)
		partialOffload = max(
			4*batch*(vocab+2*embedding),
			fullOffload,
		)
	}

	return
}
