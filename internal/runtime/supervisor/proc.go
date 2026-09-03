package supervisor

import (
	"bytes"
	"os"
	"strconv"
	"strings"
)

// descendants lists every process below root, breadth-first, by following parent links in /proc.
func descendants(root int) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	children := make(map[int][]int)
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if ppid, ok := parentOf(pid); ok {
			children[ppid] = append(children[ppid], pid)
		}
	}
	var out []int
	for queue := []int{root}; len(queue) > 0; queue = queue[1:] {
		kids := children[queue[0]]
		out = append(out, kids...)
		queue = append(queue, kids...)
	}
	return out
}

// parentOf reads the ppid field of /proc/pid/stat; the comm field may hold spaces and parens, so parse after the last ')'.
func parentOf(pid int) (int, bool) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, false
	}
	i := bytes.LastIndexByte(data, ')')
	if i < 0 {
		return 0, false
	}
	fields := strings.Fields(string(data[i+1:])) // state ppid pgrp ...
	if len(fields) < 2 {
		return 0, false
	}
	ppid, err := strconv.Atoi(fields[1])
	return ppid, err == nil
}
