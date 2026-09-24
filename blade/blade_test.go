package blade

import (
	"context"
	"testing"

	"github.com/apppackio/saw/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

// fakeClient implements CloudWatchLogsClient, returning canned pages.
type fakeClient struct {
	groupPages  [][]types.LogGroup
	streamPages [][]types.LogStream
	groupCalls  int
	streamCalls int
}

func (f *fakeClient) DescribeLogGroups(
	_ context.Context,
	_ *cloudwatchlogs.DescribeLogGroupsInput,
	_ ...func(*cloudwatchlogs.Options),
) (*cloudwatchlogs.DescribeLogGroupsOutput, error) {
	page := f.groupPages[f.groupCalls]
	f.groupCalls++
	out := &cloudwatchlogs.DescribeLogGroupsOutput{LogGroups: page}
	if f.groupCalls < len(f.groupPages) {
		out.NextToken = aws.String("more")
	}
	return out, nil
}

func (f *fakeClient) DescribeLogStreams(
	_ context.Context,
	_ *cloudwatchlogs.DescribeLogStreamsInput,
	_ ...func(*cloudwatchlogs.Options),
) (*cloudwatchlogs.DescribeLogStreamsOutput, error) {
	page := f.streamPages[f.streamCalls]
	f.streamCalls++
	out := &cloudwatchlogs.DescribeLogStreamsOutput{LogStreams: page}
	if f.streamCalls < len(f.streamPages) {
		out.NextToken = aws.String("more")
	}
	return out, nil
}

func (f *fakeClient) FilterLogEvents(
	_ context.Context,
	_ *cloudwatchlogs.FilterLogEventsInput,
	_ ...func(*cloudwatchlogs.Options),
) (*cloudwatchlogs.FilterLogEventsOutput, error) {
	return &cloudwatchlogs.FilterLogEventsOutput{}, nil
}

// A Blade built from a fake client should walk every page.
func TestGetLogGroupsPaginates(t *testing.T) {
	fake := &fakeClient{groupPages: [][]types.LogGroup{
		{{LogGroupName: aws.String("a")}, {LogGroupName: aws.String("b")}},
		{{LogGroupName: aws.String("c")}},
	}}

	b := NewBladeWithClient(fake, &config.Configuration{}, nil)
	groups := b.GetLogGroups()

	if fake.groupCalls != 2 {
		t.Errorf("expected 2 API calls (one per page), got %d", fake.groupCalls)
	}
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups across both pages, got %d", len(groups))
	}
	for i, want := range []string{"a", "b", "c"} {
		if got := aws.ToString(groups[i].LogGroupName); got != want {
			t.Errorf("group %d: expected %q, got %q", i, want, got)
		}
	}
}

func TestGetLogStreamsPaginates(t *testing.T) {
	fake := &fakeClient{streamPages: [][]types.LogStream{
		{{LogStreamName: aws.String("s1")}},
		{{LogStreamName: aws.String("s2")}},
	}}

	b := NewBladeWithClient(fake, &config.Configuration{Group: "g"}, nil)
	streams := b.GetLogStreams()

	if fake.streamCalls != 2 {
		t.Errorf("expected 2 API calls (one per page), got %d", fake.streamCalls)
	}
	if len(streams) != 2 {
		t.Fatalf("expected 2 streams, got %d", len(streams))
	}
}

// *cloudwatchlogs.Client must keep satisfying the interface.
var _ CloudWatchLogsClient = (*cloudwatchlogs.Client)(nil)

// NewBladeWithConfig should accept a v2 aws.Config directly, which is what
// callers embedding saw as a library have on hand.
func TestNewBladeWithConfig(t *testing.T) {
	b := NewBladeWithConfig(aws.Config{Region: "us-east-1"}, &config.Configuration{}, nil)
	if b == nil || b.cwl == nil {
		t.Fatal("expected a Blade with a non-nil client")
	}
}
