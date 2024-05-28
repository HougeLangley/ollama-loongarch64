package parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/stretchr/testify/assert"
)

func TestParseFileFile(t *testing.T) {
	input := `
FROM model1
ADAPTER adapter1
LICENSE MIT
PARAMETER param1 value1
PARAMETER param2 value2
TEMPLATE template1
`

	reader := strings.NewReader(input)

	modelfile, err := ParseFile(reader)
	assert.NoError(t, err)

	expectedCommands := []Command{
		{Name: "model", Args: "model1"},
		{Name: "adapter", Args: "adapter1"},
		{Name: "license", Args: "MIT"},
		{Name: "param1", Args: "value1"},
		{Name: "param2", Args: "value2"},
		{Name: "template", Args: "template1"},
	}

	assert.Equal(t, expectedCommands, modelfile.Commands)
}

func TestParseFileFrom(t *testing.T) {
	var cases = []struct {
		input    string
		expected []Command
		err      error
	}{
		{
			"FROM foo",
			[]Command{{Name: "model", Args: "foo"}},
			nil,
		},
		{
			"FROM /path/to/model",
			[]Command{{Name: "model", Args: "/path/to/model"}},
			nil,
		},
		{
			"FROM /path/to/model/fp16.bin",
			[]Command{{Name: "model", Args: "/path/to/model/fp16.bin"}},
			nil,
		},
		{
			"FROM llama3:latest",
			[]Command{{Name: "model", Args: "llama3:latest"}},
			nil,
		},
		{
			"FROM llama3:7b-instruct-q4_K_M",
			[]Command{{Name: "model", Args: "llama3:7b-instruct-q4_K_M"}},
			nil,
		},
		{
			"", nil, errMissingFrom,
		},
		{
			"PARAMETER param1 value1",
			nil,
			errMissingFrom,
		},
		{
			"PARAMETER param1 value1\nFROM foo",
			[]Command{{Name: "param1", Args: "value1"}, {Name: "model", Args: "foo"}},
			nil,
		},
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			modelfile, err := ParseFile(strings.NewReader(c.input))
			assert.ErrorIs(t, err, c.err)
			if modelfile != nil {
				assert.Equal(t, c.expected, modelfile.Commands)
			}
		})
	}
}

func TestParseFileParametersMissingValue(t *testing.T) {
	input := `
FROM foo
PARAMETER param1
`

	reader := strings.NewReader(input)

	_, err := ParseFile(reader)
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
}

func TestParseFileBadCommand(t *testing.T) {
	input := `
FROM foo
BADCOMMAND param1 value1
`
	_, err := ParseFile(strings.NewReader(input))
	assert.ErrorIs(t, err, errInvalidCommand)

}

func TestParseFileMessages(t *testing.T) {
	var cases = []struct {
		input    string
		expected []Command
		err      error
	}{
		{
			`
FROM foo
MESSAGE system You are a file parser. Always parse things.
`,
			[]Command{
				{Name: "model", Args: "foo"},
				{Name: "message", Args: "system: You are a file parser. Always parse things."},
			},
			nil,
		},
		{
			`
FROM foo
MESSAGE system You are a file parser. Always parse things.`,
			[]Command{
				{Name: "model", Args: "foo"},
				{Name: "message", Args: "system: You are a file parser. Always parse things."},
			},
			nil,
		},
		{
			`
FROM foo
MESSAGE system You are a file parser. Always parse things.
MESSAGE user Hey there!
MESSAGE assistant Hello, I want to parse all the things!
`,
			[]Command{
				{Name: "model", Args: "foo"},
				{Name: "message", Args: "system: You are a file parser. Always parse things."},
				{Name: "message", Args: "user: Hey there!"},
				{Name: "message", Args: "assistant: Hello, I want to parse all the things!"},
			},
			nil,
		},
		{
			`
FROM foo
MESSAGE system """
You are a multiline file parser. Always parse things.
"""
			`,
			[]Command{
				{Name: "model", Args: "foo"},
				{Name: "message", Args: "system: \nYou are a multiline file parser. Always parse things.\n"},
			},
			nil,
		},
		{
			`
FROM foo
MESSAGE badguy I'm a bad guy!
`,
			nil,
			errInvalidMessageRole,
		},
		{
			`
FROM foo
MESSAGE system
`,
			nil,
			io.ErrUnexpectedEOF,
		},
		{
			`
FROM foo
MESSAGE system`,
			nil,
			io.ErrUnexpectedEOF,
		},
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			modelfile, err := ParseFile(strings.NewReader(c.input))
			assert.ErrorIs(t, err, c.err)
			if modelfile != nil {
				assert.Equal(t, c.expected, modelfile.Commands)
			}
		})
	}
}

func TestParseFileQuoted(t *testing.T) {
	var cases = []struct {
		multiline string
		expected  []Command
		err       error
	}{
		{
			`
FROM foo
SYSTEM """
This is a
multiline system.
"""
			`,
			[]Command{
				{Name: "model", Args: "foo"},
				{Name: "system", Args: "\nThis is a\nmultiline system.\n"},
			},
			nil,
		},
		{
			`
FROM foo
SYSTEM """
This is a
multiline system."""
			`,
			[]Command{
				{Name: "model", Args: "foo"},
				{Name: "system", Args: "\nThis is a\nmultiline system."},
			},
			nil,
		},
		{
			`
FROM foo
SYSTEM """This is a
multiline system."""
			`,
			[]Command{
				{Name: "model", Args: "foo"},
				{Name: "system", Args: "This is a\nmultiline system."},
			},
			nil,
		},
		{
			`
FROM foo
SYSTEM """This is a multiline system."""
			`,
			[]Command{
				{Name: "model", Args: "foo"},
				{Name: "system", Args: "This is a multiline system."},
			},
			nil,
		},
		{
			`
FROM foo
SYSTEM """This is a multiline system.""
			`,
			nil,
			io.ErrUnexpectedEOF,
		},
		{
			`
FROM foo
SYSTEM "
			`,
			nil,
			io.ErrUnexpectedEOF,
		},
		{
			`
FROM foo
SYSTEM """
This is a multiline system with "quotes".
"""
`,
			[]Command{
				{Name: "model", Args: "foo"},
				{Name: "system", Args: "\nThis is a multiline system with \"quotes\".\n"},
			},
			nil,
		},
		{
			`
FROM foo
SYSTEM """"""
`,
			[]Command{
				{Name: "model", Args: "foo"},
				{Name: "system", Args: ""},
			},
			nil,
		},
		{
			`
FROM foo
SYSTEM ""
`,
			[]Command{
				{Name: "model", Args: "foo"},
				{Name: "system", Args: ""},
			},
			nil,
		},
		{
			`
FROM foo
SYSTEM "'"
`,
			[]Command{
				{Name: "model", Args: "foo"},
				{Name: "system", Args: "'"},
			},
			nil,
		},
		{
			`
FROM foo
SYSTEM """''"'""'""'"'''''""'""'"""
`,
			[]Command{
				{Name: "model", Args: "foo"},
				{Name: "system", Args: `''"'""'""'"'''''""'""'`},
			},
			nil,
		},
		{
			`
FROM foo
TEMPLATE """
{{ .Prompt }}
"""`,
			[]Command{
				{Name: "model", Args: "foo"},
				{Name: "template", Args: "\n{{ .Prompt }}\n"},
			},
			nil,
		},
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			modelfile, err := ParseFile(strings.NewReader(c.multiline))
			assert.ErrorIs(t, err, c.err)
			if modelfile != nil {
				assert.Equal(t, c.expected, modelfile.Commands)
			}
		})
	}
}

func TestParseFileParameters(t *testing.T) {
	var cases = map[string]struct {
		name, value string
	}{
		"numa true":                    {"numa", "true"},
		"num_ctx 1":                    {"num_ctx", "1"},
		"num_batch 1":                  {"num_batch", "1"},
		"num_gqa 1":                    {"num_gqa", "1"},
		"num_gpu 1":                    {"num_gpu", "1"},
		"main_gpu 1":                   {"main_gpu", "1"},
		"low_vram true":                {"low_vram", "true"},
		"f16_kv true":                  {"f16_kv", "true"},
		"logits_all true":              {"logits_all", "true"},
		"vocab_only true":              {"vocab_only", "true"},
		"use_mmap true":                {"use_mmap", "true"},
		"use_mlock true":               {"use_mlock", "true"},
		"num_thread 1":                 {"num_thread", "1"},
		"num_keep 1":                   {"num_keep", "1"},
		"seed 1":                       {"seed", "1"},
		"num_predict 1":                {"num_predict", "1"},
		"top_k 1":                      {"top_k", "1"},
		"top_p 1.0":                    {"top_p", "1.0"},
		"tfs_z 1.0":                    {"tfs_z", "1.0"},
		"typical_p 1.0":                {"typical_p", "1.0"},
		"repeat_last_n 1":              {"repeat_last_n", "1"},
		"temperature 1.0":              {"temperature", "1.0"},
		"repeat_penalty 1.0":           {"repeat_penalty", "1.0"},
		"presence_penalty 1.0":         {"presence_penalty", "1.0"},
		"frequency_penalty 1.0":        {"frequency_penalty", "1.0"},
		"mirostat 1":                   {"mirostat", "1"},
		"mirostat_tau 1.0":             {"mirostat_tau", "1.0"},
		"mirostat_eta 1.0":             {"mirostat_eta", "1.0"},
		"penalize_newline true":        {"penalize_newline", "true"},
		"stop ### User:":               {"stop", "### User:"},
		"stop ### User: ":              {"stop", "### User: "},
		"stop \"### User:\"":           {"stop", "### User:"},
		"stop \"### User: \"":          {"stop", "### User: "},
		"stop \"\"\"### User:\"\"\"":   {"stop", "### User:"},
		"stop \"\"\"### User:\n\"\"\"": {"stop", "### User:\n"},
		"stop <|endoftext|>":           {"stop", "<|endoftext|>"},
		"stop <|eot_id|>":              {"stop", "<|eot_id|>"},
		"stop </s>":                    {"stop", "</s>"},
	}

	for k, v := range cases {
		t.Run(k, func(t *testing.T) {
			var b bytes.Buffer
			fmt.Fprintln(&b, "FROM foo")
			fmt.Fprintln(&b, "PARAMETER", k)
			modelfile, err := ParseFile(&b)
			assert.NoError(t, err)

			assert.Equal(t, []Command{
				{Name: "model", Args: "foo"},
				{Name: v.name, Args: v.value},
			}, modelfile.Commands)
		})
	}
}

func TestParseFileComments(t *testing.T) {
	var cases = []struct {
		input    string
		expected []Command
	}{
		{
			`
# comment
FROM foo
	`,
			[]Command{
				{Name: "model", Args: "foo"},
			},
		},
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			modelfile, err := ParseFile(strings.NewReader(c.input))
			assert.NoError(t, err)
			assert.Equal(t, c.expected, modelfile.Commands)
		})
	}
}

func TestParseFileFormatParseFile(t *testing.T) {
	var cases = []string{
		`
FROM foo
ADAPTER adapter1
LICENSE MIT
PARAMETER param1 value1
PARAMETER param2 value2
TEMPLATE template1
MESSAGE system You are a file parser. Always parse things.
MESSAGE user Hey there!
MESSAGE assistant Hello, I want to parse all the things!
`,
		`
FROM foo
ADAPTER adapter1
LICENSE MIT
PARAMETER param1 value1
PARAMETER param2 value2
TEMPLATE template1
MESSAGE system """
You are a store greeter. Always responsed with "Hello!".
"""
MESSAGE user Hey there!
MESSAGE assistant Hello, I want to parse all the things!
`,
		`
FROM foo
ADAPTER adapter1
LICENSE """
Very long and boring legal text.
Blah blah blah.
"Oh look, a quote!"
"""

PARAMETER param1 value1
PARAMETER param2 value2
TEMPLATE template1
MESSAGE system """
You are a store greeter. Always responsed with "Hello!".
"""
MESSAGE user Hey there!
MESSAGE assistant Hello, I want to parse all the things!
`,
		`
FROM foo
SYSTEM ""
`,
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			modelfile, err := ParseFile(strings.NewReader(c))
			assert.NoError(t, err)

			modelfile2, err := ParseFile(strings.NewReader(modelfile.String()))
			assert.NoError(t, err)

			assert.Equal(t, modelfile, modelfile2)
		})
	}

}

func TestParseFileUTF16ParseFile(t *testing.T) {
	data := `FROM bob
PARAMETER param1 1
PARAMETER param2 4096
SYSTEM You are a utf16 file.
`
	// simulate a utf16 le file
	utf16File := utf16.Encode(append([]rune{'\ufffe'}, []rune(data)...))
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.LittleEndian, utf16File)
	assert.NoError(t, err)

	actual, err := ParseFile(buf)
	assert.NoError(t, err)

	expected := []Command{
		{Name: "model", Args: "bob"},
		{Name: "param1", Args: "1"},
		{Name: "param2", Args: "4096"},
		{Name: "system", Args: "You are a utf16 file."},
	}

	assert.Equal(t, expected, actual.Commands)

	// simulate a utf16 be file
	buf = new(bytes.Buffer)
	err = binary.Write(buf, binary.BigEndian, utf16File)
	assert.NoError(t, err)

	actual, err = ParseFile(buf)
	assert.NoError(t, err)
	assert.Equal(t, expected, actual.Commands)
}
