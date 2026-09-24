package blade

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/TylerBrock/colorjson"
	"github.com/TylerBrock/saw/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/fatih/color"
)

// A Blade is a Saw execution instance
type Blade struct {
	config *config.Configuration
	aws    *config.AWSConfiguration
	output *config.OutputConfiguration
	cwl    *cloudwatchlogs.Client
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
		fmt.Println("Error", err)
		os.Exit(2)
	}

	blade.cwl = cloudwatchlogs.NewFromConfig(cfg)
	blade.config = config
	blade.output = outputConfig

	return &blade
}

// GetLogGroups gets the log groups from AWS given the blade configuration
func (b *Blade) GetLogGroups() []types.LogGroup {
	input := b.config.DescribeLogGroupsInput()
	groups := make([]types.LogGroup, 0)
	paginator := cloudwatchlogs.NewDescribeLogGroupsPaginator(b.cwl, input)
	for paginator.HasMorePages() {
		out, err := paginator.NextPage(context.Background())
		if err != nil {
			fmt.Println("Error", err)
			os.Exit(2)
		}
		groups = append(groups, out.LogGroups...)
	}
	return groups
}

// GetLogStreams gets the log streams from AWS given the blade configuration
func (b *Blade) GetLogStreams() []types.LogStream {
	input := b.config.DescribeLogStreamsInput()
	streams := make([]types.LogStream, 0)
	paginator := cloudwatchlogs.NewDescribeLogStreamsPaginator(b.cwl, input)
	for paginator.HasMorePages() {
		out, err := paginator.NextPage(context.Background())
		if err != nil {
			fmt.Println("Error", err)
			os.Exit(2)
		}
		streams = append(streams, out.LogStreams...)
	}

	return streams
}

// GetEvents gets events from AWS given the blade configuration
func (b *Blade) GetEvents() {
	formatter := b.output.Formatter()
	input := b.config.FilterLogEventsInput()

	paginator := cloudwatchlogs.NewFilterLogEventsPaginator(b.cwl, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(context.Background())
		if err != nil {
			fmt.Println("Error", err)
			os.Exit(2)
		}
		for _, event := range page.Events {
			if b.output.Pretty {
				fmt.Println(formatEvent(formatter, event))
			} else {
				fmt.Println(aws.ToString(event.Message))
			}
		}
	}
}

// StreamEvents continuously prints log events to the console
func (b *Blade) StreamEvents() {
	var lastSeenTime *int64
	var seenEventIDs map[string]bool
	formatter := b.output.Formatter()
	input := b.config.FilterLogEventsInput()

	clearSeenEventIds := func() {
		seenEventIDs = make(map[string]bool, 0)
	}

	addSeenEventIDs := func(id *string) {
		seenEventIDs[*id] = true
	}

	updateLastSeenTime := func(ts *int64) {
		if lastSeenTime == nil || *ts > *lastSeenTime {
			lastSeenTime = ts
			clearSeenEventIds()
		}
	}

	handlePage := func(page *cloudwatchlogs.FilterLogEventsOutput) {
		for _, event := range page.Events {
			updateLastSeenTime(event.Timestamp)
			if _, seen := seenEventIDs[*event.EventId]; !seen {
				var message string
				if b.output.Raw {
					message = aws.ToString(event.Message)
				} else {
					message = formatEvent(formatter, event)
				}
				message = strings.TrimRight(message, "\n")
				fmt.Println(message)
				addSeenEventIDs(event.EventId)
			}
		}
	}

	for {
		paginator := cloudwatchlogs.NewFilterLogEventsPaginator(b.cwl, input)
		for paginator.HasMorePages() {
			page, err := paginator.NextPage(context.Background())
			if err != nil {
				fmt.Println("Error", err)
				os.Exit(2)
			}
			handlePage(page)
		}
		if lastSeenTime != nil {
			input.StartTime = lastSeenTime
		}
		time.Sleep(1 * time.Second)
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
