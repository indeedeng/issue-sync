package lib

import (
	"github.com/indeedeng/issue-sync/lib/issuesyncgithub"
	"github.com/indeedeng/issue-sync/lib/issuesyncjira"
	"regexp"
	"strconv"

	"github.com/andygrunwald/go-jira"
	"github.com/indeedeng/issue-sync/cfg"
	"github.com/google/go-github/v28/github"
)

// jCommentRegex matches a generated JIRA comment. It has matching groups to retrieve the
// GitHub Comment ID (\1), the GitHub username (\2), the GitHub real name (\3, if it exists),
// the time the comment was posted (\3 or \4), and the body of the comment (\4 or \5).
var jCommentRegex = regexp.MustCompile("^Comment \\[\\(ID (\\d+)\\)\\|.*?] from GitHub user \\[(.+)\\|.*?] \\((.+)\\) at (.+):\\n\\n(?s)(.+)$")

// jCommentIDRegex just matches the beginning of a generated JIRA comment. It's a smaller,
// simpler, and more efficient regex, to quickly filter only generated comments and retrieve
// just their GitHub ID for matching.
var jCommentIDRegex = regexp.MustCompile("^Comment \\[\\(ID (\\d+)\\)\\|")

// CompareComments takes a GitHub issue, and retrieves all of its comments. It then
// matches each one to a comment in `existing`. If it finds a match, it calls
// UpdateComment; if it doesn't, it calls CreateComment.
func CompareComments(config cfg.Config, ghIssue github.Issue, jIssue jira.Issue, ghClient issuesyncgithub.Client, jClient issuesyncjira.Client) error {
	log := config.GetLogger()
	user, repoName := config.GetRepo()

	if ghIssue.GetComments() == 0 {
		log.Debugf("Issue #%d has no comments, skipping.", *ghIssue.Number)
		return nil
	}

	ghComments, err := issuesyncgithub.ListComments(ghClient, config.GetTimeout(), user, repoName, ghIssue)
	if err != nil {
		return err
	}

	var jComments []jira.Comment
	if jIssue.Fields.Comments == nil {
		log.Debugf("JIRA issue %s has no comments.", jIssue.Key)
	} else {
		commentPtrs := jIssue.Fields.Comments.Comments
		jComments = make([]jira.Comment, len(commentPtrs))
		for i, v := range commentPtrs {
			jComments[i] = *v
		}
		log.Debugf("JIRA issue %s has %d comments", jIssue.Key, len(jComments))
	}

	for _, ghComment := range ghComments {
		found := false
		for _, jComment := range jComments {
			if !jCommentIDRegex.MatchString(jComment.Body) {
				continue
			}
			// matches[0] is the whole string, matches[1] is the ID
			matches := jCommentIDRegex.FindStringSubmatch(jComment.Body)
			id, err := strconv.ParseInt(matches[1], 10, 64)

			if err != nil {
				log.Error(err)
				continue
			}

			if *ghComment.ID != id {
				continue
			}

			found = true

			err = UpdateComment(config, *ghComment, jComment, jIssue, ghClient, jClient)
			if err != nil {
				log.Error(err)
			}

			break
		}
		if found {
			continue
		}

		comment, err := issuesyncjira.CreateComment(jClient, config.GetTimeout(), jIssue, *ghComment, ghClient)
		if err != nil {
			return err
		}

		log.Debugf("Created JIRA comment %s.", comment.ID)
	}

	log.Debugf("Copied comments from GH issue #%d to JIRA issue %s.", *ghIssue.Number, jIssue.Key)
	return nil
}

// UpdateComment compares the body of a GitHub comment with the body (minus header)
// of the JIRA comment, and updates the JIRA comment if necessary.
func UpdateComment(config cfg.Config, ghComment github.IssueComment, jComment jira.Comment, jIssue jira.Issue, ghClient issuesyncgithub.Client, jClient issuesyncjira.Client) error {
	log := config.GetLogger()

	// fields[0] is the whole body, 1 is the ID, 2 is the username, 3 is the real name (or "" if none)
	// 4 is the date, and 5 is the real body
	fields := jCommentRegex.FindStringSubmatch(jComment.Body)

	if fields[5] == ghComment.GetBody() {
		return nil
	}

	comment, err := issuesyncjira.UpdateComment(jClient, config.GetTimeout(), jIssue, jComment.ID, ghComment, ghClient)
	if err != nil {
		return err
	}

	log.Debugf("Updated JIRA comment %s.", comment.ID)

	return nil
}
