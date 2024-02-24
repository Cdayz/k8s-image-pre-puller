package names

import (
	"bytes"
	"fmt"
	"hash/crc32"
	"math"
	"regexp"
	"sort"
	"strings"
)

type k8sNameOpts struct {
	maxLength  int
	includeCRC bool
}

const (
	hashLength        = 10
	defaultNameLength = 63
)

type NameOption func(*k8sNameOpts)

func MaxLength(length int) NameOption {
	return func(opts *k8sNameOpts) { opts.maxLength = length }
}

func IncludeCRC(flag bool) NameOption {
	return func(opts *k8sNameOpts) { opts.includeCRC = flag }
}

func MakeK8SName(rawNames []string, optFuncs ...NameOption) string {
	if len(rawNames) == 0 {
		return ""
	}

	opts := k8sNameOpts{
		maxLength:  defaultNameLength,
		includeCRC: true,
	}
	for _, op := range optFuncs {
		op(&opts)
	}

	h := StringHash(strings.Join(rawNames, ""))

	totalLength := 0
	var names []string
	for _, rn := range rawNames {
		name := removeNotK8SCompliantChars(rn)
		if name != rn {
			// If name was changed, we should include crc hashsum
			opts.includeCRC = true
		}
		totalLength += len(name)
		names = append(names, name)
	}

	validLength := opts.maxLength - hashLength - len(names)
	if totalLength > validLength {
		opts.includeCRC = true
		// have to reduce names lengths
		names = reduceNamesLengths(names, validLength)
	}

	nameWithoutCRC := strings.Join(names, "-")

	if opts.includeCRC {
		return fmt.Sprintf("%s-%d", nameWithoutCRC, h)
	}

	return nameWithoutCRC
}

// first and last symbols should be alphanumeric, but also can be an - symbol inside
var k8sCompliantRe = regexp.MustCompile(`(^[^a-z0-9]+)|([^a-z0-9\-]+)|([^a-z0-9]+$)`)

func removeNotK8SCompliantChars(name string) string {
	return k8sCompliantRe.ReplaceAllString(strings.ToLower(name), "")
}

func reduceNamesLengths(names []string, validLength int) []string {
	n := int(math.Floor(float64(validLength) / float64(len(names))))

	var res []string
	for _, name := range names {
		r := name
		if len(r) > n {
			r = r[:n]
		}
		res = append(res, r)
	}

	return res
}

func StringHash(str string) uint32 {
	return crc32.ChecksumIEEE([]byte(str))
}

func StringMapHash(m map[string]string) uint32 {
	type kv struct {
		k string
		v string
	}

	kvs := []kv{}
	for k, v := range m {
		kvs = append(kvs, kv{k: k, v: v})
	}

	sort.SliceStable(kvs, func(i, j int) bool { return kvs[i].k < kvs[j].k })

	buf := new(bytes.Buffer)
	for _, pair := range kvs {
		buf.WriteString(pair.k)
		buf.WriteString(":")
		buf.WriteString(pair.v)
		buf.WriteString(";")
	}

	return crc32.ChecksumIEEE(buf.Bytes())
}
