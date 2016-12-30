// ahocorasick.go: implementation of the Aho-Corasick string matching
// algorithm. Actually implemented as matching against []byte rather
// than the Go string type. Throughout this code []byte is referred to
// as a blice.
//
// http://en.wikipedia.org/wiki/Aho%E2%80%93Corasick_string_matching_algorithm
//
// Copyright (c) 2013 CloudFlare, Inc.

package ahocorasick

import (
	"bytes"
	"container/list"
	"errors"
	"sort"
	"strings"
	"unicode/utf8"
)

// A node in the trie structure used to implement Aho-Corasick
type node struct {
	root bool // true if this is the root

	b []byte // The blice at this node

	output bool // True means this node represents a blice that should
	// be output when matching
	index int // index into original dictionary if output is true

	counter int // Set to the value of the Matcher.counter when a
	// match is output to prevent duplicate output

	// The use of fixed size arrays is space-inefficient but fast for
	// lookups.

	child [256]*node // A non-nil entry in this array means that the
	// index represents a byte value which can be
	// appended to the current node. Blices in the
	// trie are built up byte by byte through these
	// child node pointers.

	fails [256]*node // Where to fail to (by following the fail
	// pointers) for each possible byte

	suffix *node // Pointer to the longest possible strict suffix of
	// this node

	fail *node // Pointer to the next node which is in the dictionary
	// which can be reached from here following suffixes. Called fail
	// because it is used to fallback in the trie when a match fails.
}

// Matcher is returned by NewMatcher and contains a list of blices to
// match against
type Matcher struct {
	counter int // Counts the number of matches done, and is used to
	// prevent output of multiple matches of the same string
	trie []node // preallocated block of memory containing all the
	// nodes
	extent     int   // offset into trie that is currently free
	root       *node // Points to trie[0]
	dictionary [][]byte
}

// finndBlice looks for a blice in the trie starting from the root and
// returns a pointer to the node representing the end of the blice. If
// the blice is not found it returns nil.
func (m *Matcher) findBlice(b []byte) *node {
	n := &m.trie[0]

	for n != nil && len(b) > 0 {
		n = n.child[int(b[0])]
		b = b[1:]
	}

	return n
}

// getFreeNode: gets a free node structure from the Matcher's trie
// pool and updates the extent to point to the next free node.
func (m *Matcher) getFreeNode() *node {
	m.extent += 1

	if m.extent == 1 {
		m.root = &m.trie[0]
		m.root.root = true
	}

	return &m.trie[m.extent-1]
}

// buildTrie builds the fundamental trie structure from a set of
// blices.
func (m *Matcher) buildTrie(dictionary [][]byte) {

	// Work out the maximum size for the trie (all dictionary entries
	// are distinct plus the root). This is used to preallocate memory
	// for it.

	max := 1
	for _, blice := range dictionary {
		max += len(blice)
	}
	m.trie = make([]node, max)

	// Calling this an ignoring its argument simply allocated
	// m.trie[0] which will be the root element

	m.getFreeNode()

	// This loop builds the nodes in the trie by following through
	// each dictionary entry building the children pointers.

	for i, blice := range dictionary {
		n := m.root
		var path []byte
		for _, b := range blice {
			path = append(path, b)

			c := n.child[int(b)]

			if c == nil {
				c = m.getFreeNode()
				n.child[int(b)] = c
				c.b = make([]byte, len(path))
				copy(c.b, path)

				// Nodes directly under the root node will have the
				// root as their fail point as there are no suffixes
				// possible.

				if len(path) == 1 {
					c.fail = m.root
				}

				c.suffix = m.root
			}

			n = c
		}

		// The last value of n points to the node representing a
		// dictionary entry

		n.output = true
		n.index = i
	}

	l := new(list.List)
	l.PushBack(m.root)

	for l.Len() > 0 {
		n := l.Remove(l.Front()).(*node)

		for i := 0; i < 256; i++ {
			c := n.child[i]
			if c != nil {
				l.PushBack(c)

				for j := 1; j < len(c.b); j++ {
					c.fail = m.findBlice(c.b[j:])
					if c.fail != nil {
						break
					}
				}

				if c.fail == nil {
					c.fail = m.root
				}

				for j := 1; j < len(c.b); j++ {
					s := m.findBlice(c.b[j:])
					if s != nil && s.output {
						c.suffix = s
						break
					}
				}
			}
		}
	}

	for i := 0; i < m.extent; i++ {
		for c := 0; c < 256; c++ {
			n := &m.trie[i]
			for n.child[c] == nil && !n.root {
				n = n.fail
			}

			m.trie[i].fails[c] = n
		}
	}

	m.trie = m.trie[:m.extent]
}

// NewMatcher creates a new Matcher used to match against a set of
// blices
func NewMatcher(dictionary [][]byte) *Matcher {
	m := new(Matcher)

	m.buildTrie(dictionary)
	m.dictionary = dictionary
	return m
}

// NewStringMatcher creates a new Matcher used to match against a set
// of strings (this is a helper to make initialization easy)
func NewStringMatcher(dictionary []string) *Matcher {
	m := new(Matcher)

	var d [][]byte
	for _, s := range dictionary {
		d = append(d, []byte(s))
	}

	m.buildTrie(d)
	m.dictionary = d
	return m
}

// Match searches in for blices and returns all the blices found as
// indexes into the original dictionary
func (m *Matcher) Match(in []byte) []int {
	m.counter += 1
	var hits []int

	n := m.root

	for _, b := range in {
		c := int(b)

		if !n.root && n.child[c] == nil {
			n = n.fails[c]
		}

		if n.child[c] != nil {
			f := n.child[c]
			n = f

			if f.output && f.counter != m.counter {
				hits = append(hits, f.index)
				f.counter = m.counter
			}

			for !f.suffix.root {
				f = f.suffix
				if f.counter != m.counter {
					hits = append(hits, f.index)
					f.counter = m.counter
				} else {

					// There's no point working our way up the
					// suffixes if it's been done before for this call
					// to Match. The matches are already in hits.

					break
				}
			}
		}
	}

	return hits
}

func (m *Matcher) Replace(inStr string, replacerStr string, isReplace bool, hitType int) (string, interface{}, error) {

	in := []byte(inStr)

	replacer := []byte(replacerStr)
	var out []byte

	m.counter += 1
	var hitsWord []string
	var hitsWordCount map[string]int64
	var hitsWordIndex map[string][]int64
	var hitsIndexWord map[int64]string
	var hits map[int][]byte
	hits = make(map[int][]byte)
	if hitType == EnumHitTypeNone {
		if !isReplace {
			return "", nil, errors.New("match not support hittype none")
		}
	} else if hitType == EnumHitTypeWord {

	} else if hitType == EnumHitTypeWordCount {
		hitsWordCount = make(map[string]int64)
	} else if hitType == EnumHitTypeWordIndex {
		hitsWordIndex = make(map[string][]int64)
	} else if hitType == EnumHitTypeIndexWord {
		hitsIndexWord = make(map[int64]string)
	} else {
		return "", nil, errors.New("hit type not support")
	}

	n := m.root

	var bstr string

	for i, b := range in {
		c := int(b)

		if !n.root && n.child[c] == nil {
			n = n.fails[c]
		}

		if n.child[c] != nil {

			f := n.child[c]
			n = f
			if f.output && isReplace {
				hits[i-len(f.b)+1] = f.b
			}
			if f.output && (hitType != EnumHitTypeWord || f.counter != m.counter) {

				if hitType == EnumHitTypeWord {
					hitsWord = append(hitsWord, string(f.b))
				} else if hitType == EnumHitTypeWordCount {
					bstr = string(f.b)
					if _, ok := hitsWordCount[bstr]; !ok {
						hitsWordCount[bstr] = 0
					}
					hitsWordCount[bstr] += 1

				} else if hitType == EnumHitTypeWordIndex {
					bstr = string(f.b)

					hitsWordIndex[bstr] = append(hitsWordIndex[bstr], int64(utf8.RuneCount(in[:i+1])-utf8.RuneCount(f.b)))
				} else if hitType == EnumHitTypeIndexWord {

					hitsIndexWord[int64(utf8.RuneCount(in[:i+1])-utf8.RuneCount(f.b))] = string(f.b)
				}

				f.counter = m.counter

			}
			for !f.suffix.root {
				f = f.suffix
				if f.output && isReplace {
					hits[i-len(f.b)+1] = f.b
				}

				if hitType != EnumHitTypeWord || f.counter != m.counter {
					if hitType == EnumHitTypeWord {
						hitsWord = append(hitsWord, string(f.b))
					} else if hitType == EnumHitTypeWordCount {
						bstr = string(f.b)
						if _, ok := hitsWordCount[bstr]; !ok {
							hitsWordCount[bstr] = 0
						}
						hitsWordCount[bstr] += 1

					} else if hitType == EnumHitTypeWordIndex {
						bstr = string(f.b)

						hitsWordIndex[bstr] = append(hitsWordIndex[bstr], int64(utf8.RuneCount(in[:i+1])-utf8.RuneCount(f.b)))
					} else if hitType == EnumHitTypeIndexWord {

						hitsIndexWord[int64(utf8.RuneCount(in[:i+1])-utf8.RuneCount(f.b))] = string(f.b)
					}

					f.counter = m.counter

				} else {

					// There's no point working our way up the
					// suffixes if it's been done before for this call
					// to Match. The matches are already in hits.

					break
				}
			}

		}

	}
	if isReplace {

		var lastIndex int

		keys := make([]int, 0, len(hits))
		for k := range hits {
			keys = append(keys, k)
		}

		sort.Ints(keys)

		for _, index := range keys {
			word := hits[index]
			if index > lastIndex {
				out = append(out, in[lastIndex:index]...)
			}

			lenWord := utf8.RuneCount(word)

			for i := 0; i < lenWord; i++ {
				out = append(out, replacer...)
			}

			lastIndex = index + len(word)

		}
		if lastIndex < len(in)-1 {
			out = append(out, in[lastIndex:]...)
		}
	} else {
		out = []byte{}
	}
	//copy(out, in)

	var iHits interface{}

	if hitType == EnumHitTypeWord {
		iHits = hitsWord
	} else if hitType == EnumHitTypeWordCount {
		iHits = hitsWordCount
	} else if hitType == EnumHitTypeWordIndex {
		iHits = hitsWordIndex
	} else if hitType == EnumHitTypeIndexWord {
		iHits = hitsIndexWord
	}

	return string(out), iHits, nil
}

//benchmark
func (m *Matcher) Re2(in string, replacer string) string {
	hits := m.Match([]byte(in))
	if len(hits) > 0 {
		for _, h := range hits {
			word := m.dictionary[h]
			var bb bytes.Buffer
			for i := 0; i < utf8.RuneCount(word); i++ {
				bb.WriteString(string(replacer))
			}
			in = strings.Replace(in, string(word), bb.String(), -1)
		}
	}
	return in
}
