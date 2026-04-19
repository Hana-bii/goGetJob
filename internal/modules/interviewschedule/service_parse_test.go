package interviewschedule

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"goGetJob/internal/common/ai"
)

func TestParseService_UsesRegexWhenInviteHasScheduleFields(t *testing.T) {
	service := NewParseService(nil)

	result, err := service.Parse(context.Background(), ParseRequest{
		RawText: "公司：深蓝科技\n岗位：Go 开发工程师\n时间：2026-04-20 14:30\n链接：https://meeting.feishu.cn/abc123\n第2轮\n面试官：张三",
		Source:  "feishu",
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "rule", result.ParseMethod)
	require.Equal(t, 0.95, result.Confidence)
	require.NotNil(t, result.Data)
	require.Equal(t, "深蓝科技", result.Data.CompanyName)
	require.Equal(t, "Go 开发工程师", result.Data.Position)
	require.Equal(t, "https://meeting.feishu.cn/abc123", result.Data.MeetingLink)
	require.Equal(t, "张三", result.Data.Interviewer)
	require.Equal(t, 2, result.Data.RoundNumber)
	require.Equal(t, "2026-04-20T14:30:00", result.Data.InterviewTime)
	require.Equal(t, "VIDEO", result.Data.InterviewType)
}

func TestParseService_UsesAIFallbackWhenRegexCannotParse(t *testing.T) {
	service := NewParseService(fakeChatModel{
		response: `{"companyName":"Acme","position":"Go Engineer","interviewTime":"2026-04-20T14:30:00","interviewType":"VIDEO","meetingLink":"https://zoom.us/j/123","roundNumber":3,"interviewer":"李四","notes":"线上面试"}`,
	})

	result, err := service.Parse(context.Background(), ParseRequest{
		RawText: "please schedule an interview for the candidate",
		Source:  "other",
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "ai", result.ParseMethod)
	require.Equal(t, 0.8, result.Confidence)
	require.NotNil(t, result.Data)
	require.Equal(t, "Acme", result.Data.CompanyName)
	require.Equal(t, "Go Engineer", result.Data.Position)
	require.Equal(t, "2026-04-20T14:30:00", result.Data.InterviewTime)
	require.Equal(t, "https://zoom.us/j/123", result.Data.MeetingLink)
	require.Equal(t, 3, result.Data.RoundNumber)
	require.Equal(t, "李四", result.Data.Interviewer)
	require.Equal(t, "线上面试", result.Data.Notes)
}

func TestParseServiceRejectsUnsupportedInterviewTypeFromAI(t *testing.T) {
	service := NewParseService(fakeChatModel{
		response: `{"companyName":"Acme","position":"Go Engineer","interviewTime":"2026-04-20T14:30:00","interviewType":"WEBEX","meetingLink":"https://zoom.us/j/123","roundNumber":3,"interviewer":"李四","notes":"线上面试"}`,
	})

	result, err := service.Parse(context.Background(), ParseRequest{
		RawText: "please schedule an interview for the candidate",
		Source:  "other",
	})
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, "none", result.ParseMethod)
	require.Contains(t, result.Log, "unsupported interview type")
}

type fakeChatModel struct {
	response string
	err      error
}

func (f fakeChatModel) Generate(context.Context, []ai.ChatMessage) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.response, nil
}
