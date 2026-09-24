package blade

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/TylerBrock/colorjson"
	"github.com/apppackio/saw/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/fatih/color"
)

// CloudWatchLogsClient is the subset of the CloudWatch Logs API that a Blade
// uses. *cloudwatchlogs.Client satisfies it, and so can a fake, which makes
// a Blade testable without reaching AWS.
type CloudWatchLogsClient interface {
	cloudwatchlogs.DescribeLogGroupsAPIClient
	cloudwatchlogs.DescribeLogStreamsAPIClient
	cloudwatchlogs.FilterLogEventsAPIClient
}

// A Blade is a Saw execution instance
type Blade struct {
	config *config.Configuration
	aws    *config.AWSConfiguration
	output *config.OutputConfiguration
	cwl    CloudWatchLogsClient
}

// NewBlade creates a new Blade with CloudWatchLogs instance from provided config
func NewBlade(
	config *config.Configuration,
	awsConfig *config.AWSConfiguration,
	outputConfig *config.OutputConfiguration,
) *Blade {
	blade := Blade{}

	loadOpts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithAssumeRoleCredentialOptions(func(o *stscreds.AssumeRoleOptions) {
			o.TokenProvider = stscreds.StdinTokenProvider
		}),
	}

	if awsConfig.Region != "" {
		loadOpts = append(loadOpts, awsconfig.WithRegion(awsConfig.Region))
	}

	if awsConfig.Profile != "" {
		loadOpts = append(loadOpts, awsconfig.WithSharedConfigProfile(awsConfig.Profile))
	}

	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), loadOpts...)
	if err != nil {
		exit(err)
	}

	blade.cwl = cloudwatchlogs.NewFromConfig(cfg)
	blade.config = config
	blade.output = outputConfig

	return &blade
}

// NewBladeWithClient creates a Blade backed by an existing CloudWatch Logs
// client, rather than building one from the ambient AWS configuration the way
// NewBlade does. Use it when embedding saw in another program that already has
// a configured client, or to supply a fake in tests.
//
// outputConfig may be nil for the commands that do not format events
// (GetLogGroups and GetLogStreams).
func NewBladeWithClient(
	cwl CloudWatchLogsClient,
	config *config.Configuration,
	outputConfig *config.OutputConfiguration,
) *Blade {
	return &Blade{
		cwl:    cwl,
		config: config,
		output: outputConfig,
	}
}

// NewBladeWithConfig creates a Blade from an existing aws.Config, rather than
// loading one from the environment the way NewBlade does. Use it when
// embedding saw in a program that has already built its AWS configuration.
//
// outputConfig may be nil for the commands that do not format events
// (GetLogGroups and GetLogStreams).
func NewBladeWithConfig(
	awsCfg aws.Config,
	config *config.Configuration,
	outputConfig *config.OutputConfiguration,
) *Blade {
	return NewBladeWithClient(cloudwatchlogs.NewFromConfig(awsCfg), config, outputConfig)
}

// exit reports err the way the CLI always has and terminates. Only the
// CLI-facing wrappers below call it; the library methods return errors.
func exit(err error) {
	fmt.Println("Error", err)
	os.Exit(2)
}

// LogGroups returns every log group matching the blade configuration, walking
// all pages. It is the error-returning form of GetLogGroups.
func (b *Blade) LogGroups(ctx context.Context) ([]types.LogGroup, error) {
	input := b.config.DescribeLogGroupsInput()
	groups := make([]types.LogGroup, 0)
	paginator := cloudwatchlogs.NewDescribeLogGroupsPaginator(b.cwl, input)
	for paginator.HasMorePages() {
		out, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		groups = append(groups, out.LogGroups...)
	}
	return groups, nil
}

// GetLogGroups gets the log groups from AWS given the blade configuration.
//
// On failure it prints the error and exits the process, which suits the CLI
// but not a library. Embedders should call LogGroups instead.
func (b *Blade) GetLogGroups() []types.LogGroup {
	groups, err := b.LogGroups(context.Background())
	if err != nil {
		exit(err)
	}
	return groups
}

// LogStreams returns every log stream matching the blade configuration,
// walking all pages. It is the error-returning form of GetLogStreams.
func (b *Blade) LogStreams(ctx context.Context) ([]types.LogStream, error) {
	input := b.config.DescribeLogStreamsInput()
	streams := make([]types.LogStream, 0)
	paginator := cloudwatchlogs.NewDescribeLogStreamsPaginator(b.cwl, input)
	for paginator.HasMorePages() {
		out, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		streams = append(streams, out.LogStreams...)
	}
	return streams, nil
}

// GetLogStreams gets the log streams from AWS given the blade configuration.
//
// On failure it prints the error and exits the process, which suits the CLI
// but not a library. Embedders should call LogStreams instead.
func (b *Blade) GetLogStreams() []types.LogStream {
	streams, err := b.LogStreams(context.Background())
	if err != nil {
		exit(err)
	}
	return streams
}

// Events calls fn once for each event matching the blade configuration,
// walking all pages. If fn returns an error, iteration stops and Events
// returns that error. It is the error-returning form of GetEvents, and hands
// the caller the events themselves rather than printing them.
func (b *Blade) Events(ctx context.Context, fn func(types.FilteredLogEvent) error) error {
	input := b.config.FilterLogEventsInput()
	paginator := cloudwatchlogs.NewFilterLogEventsPaginator(b.cwl, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, event := range page.Events {
			if err := fn(event); err != nil {
				return err
			}
		}
	}
	return nil
}

// GetEvents gets events from AWS given the blade configuration and prints
// them to stdout.
//
// On failure it prints the error and exits the process, which suits the CLI
// but not a library. Embedders should call Events instead.
func (b *Blade) GetEvents() {
	formatter := b.output.Formatter()
	err := b.Events(context.Background(), func(event types.FilteredLogEvent) error {
		if b.output.Pretty {
			fmt.Println(formatEvent(formatter, event))
		} else {
			fmt.Println(aws.ToString(event.Message))
		}
		return nil
	})
	if err != nil {
		exit(err)
	}
}

// Stream polls CloudWatch Logs and calls fn for each event it has not already
// delivered, advancing the window as newer events arrive. It returns when ctx
// is cancelled, when fn returns an error, or when a request fails; a cancelled
// ctx yields ctx.Err(). It is the error-returning form of StreamEvents, and
// hands the caller the events themselves rather than printing them.
func (b *Blade) Stream(ctx context.Context, fn func(types.FilteredLogEvent) error) error {
	var lastSeenTime *int64
	seenEventIDs := make(map[string]bool)
	input := b.config.FilterLogEventsInput()

	updateLastSeenTime := func(ts *int64) {
		if ts == nil {
			return
		}
		if lastSeenTime == nil || *ts > *lastSeenTime {
			lastSeenTime = ts
			seenEventIDs = make(map[string]bool)
		}
	}

	for {
		paginator := cloudwatchlogs.NewFilterLogEventsPaginator(b.cwl, input)
		for paginator.HasMorePages() {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				return err
			}
			for _, event := range page.Events {
				updateLastSeenTime(event.Timestamp)
				id := aws.ToString(event.EventId)
				if seenEventIDs[id] {
					continue
				}
				if err := fn(event); err != nil {
					return err
				}
				seenEventIDs[id] = true
			}
		}
		if lastSeenTime != nil {
			input.StartTime = lastSeenTime
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}

// StreamEvents continuously prints log events to the console. It does not
// return.
//
// On failure it prints the error and exits the process, which suits the CLI
// but not a library. Embedders should call Stream instead.
func (b *Blade) StreamEvents() {
	formatter := b.output.Formatter()
	err := b.Stream(context.Background(), func(event types.FilteredLogEvent) error {
		var message string
		if b.output.Raw {
			message = aws.ToString(event.Message)
		} else {
			message = formatEvent(formatter, event)
		}
		fmt.Println(strings.TrimRight(message, "\n"))
		return nil
	})
	if err != nil {
		exit(err)
	}
}

// formatEvent returns a CloudWatch log event as a formatted string using the provided formatter
func formatEvent(formatter *colorjson.Formatter, event types.FilteredLogEvent) string {
	red := color.New(color.FgRed).SprintFunc()
	white := color.New(color.FgWhite).SprintFunc()

	str := aws.ToString(event.Message)
	bytes := []byte(str)
	date := time.UnixMilli(aws.ToInt64(event.Timestamp))
	dateStr := date.Format(time.RFC3339)
	streamStr := aws.ToString(event.LogStreamName)
	jl := map[string]interface{}{}

	if err := json.Unmarshal(bytes, &jl); err != nil {
		return fmt.Sprintf("[%s] (%s) %s", red(dateStr), white(streamStr), str)
	}

	output, _ := formatter.Marshal(jl)
	return fmt.Sprintf("[%s] (%s) %s", red(dateStr), white(streamStr), output)
}
