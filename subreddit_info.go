package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"go.bbkane.com/warg/value/contained"
)

type SubredditInfo struct {
	Subreddit string
	Timeframe string
	Count     int
}

var validTimeFrames = map[string]bool{
	"day":   true,
	"week":  true,
	"month": true,
	"year":  true,
	"all":   true,
}

func FromString(s string) (SubredditInfo, error) {
	// Expected format: <subreddit>,<day|week|month|year>,<count>
	parts := strings.Split(s, ",")
	if len(parts) != 3 {
		return SubredditInfo{}, fmt.Errorf("invalid format for SubredditInfo: %s", s)
	}
	count, err := strconv.Atoi(parts[2])
	if err != nil {
		return SubredditInfo{}, fmt.Errorf("invalid count in SubredditInfo: %s", parts[2])
	}
	timeFrame := parts[1]
	if !validTimeFrames[timeFrame] {
		return SubredditInfo{}, fmt.Errorf("invalid timeframe in SubredditInfo: %s", timeFrame)
	}
	return SubredditInfo{
		Subreddit: parts[0],
		Timeframe: timeFrame,
		Count:     count,
	}, nil

}

func FromIFace(iFace interface{}) (SubredditInfo, error) {
	m, ok := iFace.(map[string]interface{})
	if !ok {
		return SubredditInfo{}, fmt.Errorf("expected map[string]interface{}, got %T", iFace)
	}
	subreddit, ok := m["name"].(string)
	if !ok {
		return SubredditInfo{}, fmt.Errorf("expected subreddit to be string, got %T", m["subreddit"])
	}
	timeframe, ok := m["timeframe"].(string)
	if !ok {
		return SubredditInfo{}, fmt.Errorf("expected timeframe to be string, got %T", m["timeframe"])
	}
	if !validTimeFrames[timeframe] {
		return SubredditInfo{}, fmt.Errorf("invalid timeframe in SubredditInfo: %s", timeframe)
	}
	count, ok := m["limit"].(uint64) // YAML numbers are decoded as uint64
	if !ok {
		return SubredditInfo{}, fmt.Errorf("expected count to be uint64, got %T", m["count"])
	}
	if count > math.MaxInt {
		return SubredditInfo{}, fmt.Errorf("count too large: %d", count)
	}
	return SubredditInfo{
		Subreddit: subreddit,
		Timeframe: timeframe,
		Count:     int(count),
	}, nil
}

func SubredditInfoTypeInfo() contained.TypeInfo[SubredditInfo] {
	return contained.TypeInfo[SubredditInfo]{
		Description: "SubredditInfo represents a subreddit, timeframe, and count",
		FromIFace:   FromIFace,
		FromString:  FromString,
		FromZero:    contained.FromZero[SubredditInfo],
		Equals:      contained.Equals[SubredditInfo],
	}
}
