package mcp

import s "github.com/cocofhu/approving/internal/mcp/structured"

// Re-export structured product symbols so existing importers keep working.

const (
	ClarifiedRequirementArtifactName = s.ClarifiedRequirementArtifactName
	ResearchArtifactName             = s.ResearchArtifactName
	ProposalsArtifactName            = s.ProposalsArtifactName
	ProposalArtifactName             = s.ProposalArtifactName
	TestResultArtifactName           = s.TestResultArtifactName
	ReviewArtifactName               = s.ReviewArtifactName
	ImplementationResultArtifactName = s.ImplementationResultArtifactName
)

type ProposalChoice = s.ProposalChoice

var (
	ClarifiedOpenQuestions             = s.ClarifiedOpenQuestions
	RenderClarifiedRequirementMarkdown = s.RenderClarifiedRequirementMarkdown
	RenderResearchMarkdown             = s.RenderResearchMarkdown
	RenderProposalsMarkdown            = s.RenderProposalsMarkdown
	RenderProposalMarkdown             = s.RenderProposalMarkdown
	RenderTestResultMarkdown           = s.RenderTestResultMarkdown
	RenderReviewMarkdown               = s.RenderReviewMarkdown
	RenderImplementationResultMarkdown = s.RenderImplementationResultMarkdown
	ProposalChoices                    = s.ProposalChoices
	SelectProposal                     = s.SelectProposal
	TestFailedCount                    = s.TestFailedCount
	TestSkippedCount                   = s.TestSkippedCount
	ReviewVerdict                      = s.ReviewVerdict
	ReviewVerdictOK                    = s.ReviewVerdictOK
)

const MinimalValidClarifiedRequirementJSON = s.MinimalValidClarifiedRequirementJSON
