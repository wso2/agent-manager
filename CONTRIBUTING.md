# Contributing Guidelines

This document establishes guidelines for using GitHub Discussions and Issues for technical conversations about the AI Agent Management Platform.

## Repository

| Repository | Visibility | Purpose |
|------------|------------|---------|
| [wso2/ai-agent-management-platform](https://github.com/wso2/ai-agent-management-platform) | **Public** | Open source version, default location for discussions and issues |

### Contribution Policy

- **All welcome**: Discussions and issues from the community are encouraged
- **Open collaboration**: Feature ideas, bug reports, and design proposals are all welcome
- **Security issues**: Please report security vulnerabilities to security@wso2.com as per the [WSO2 Security Reporting Guidelines](https://security.docs.wso2.com/en/latest/security-reporting/report-security-issues/)

## Discussion Categories

| Category | Purpose | Example Topics |
|----------|---------|----------------|
| **Announcements** | Official updates from maintainers | Releases, roadmap updates, breaking changes |
| **General** | Open-ended conversations | Community introductions, general questions |
| **Ideas** | Feature suggestions and brainstorming | New capabilities, integration ideas |
| **Q&A** | Technical questions with answers | Implementation help, troubleshooting |
| **Show and Tell** | Share projects and integrations | Agent implementations, use cases |
| **Design Proposals** | Technical design discussions | Architecture changes, system design, new features requiring review |

## When to Use Discussions vs Issues

| Use Discussions For | Use Issues For |
|---------------------|----------------|
| Open-ended questions | Bug reports with reproduction steps |
| Feature ideas and brainstorming | Concrete feature requests with clear scope |
| Design proposals and RFCs | Actionable tasks and work items |
| Community engagement | Pull request discussions |
| Troubleshooting help | Security vulnerabilities (private) |

## Guidelines

### Starting a Discussion

1. **Search first** - Check existing discussions to avoid duplicates
2. **Choose the right category** - Use the category table above
3. **Use a clear title** - Be specific and descriptive
4. **Provide context** - Include relevant details, code snippets, or diagrams

### Promoting Discussions to Issues

When a discussion results in actionable work:
1. Summarize the outcome in a final comment
2. Create a linked GitHub Issue for implementation
3. Reference the discussion in the issue for context

## Feature Lifecycle

Features progress through distinct stages from initial concept to implementation:

### 1. Idea Stage

High-level discussions about capabilities we want to explore start in the **Ideas** category. These are similar to epics—broad in scope with no imposed structure. Ideas allow open brainstorming before committing to specific solutions.

### 2. Design Proposal Stage

When an idea is refined into a well-scoped feature, create a discussion in the **Design Proposals** category. Proposals must follow the standard template:

| Section | Description |
|---------|-------------|
| **Problem** | Describe the problem, who is affected, and the impact |
| **User Stories** | Define user stories using the format "As a [role], I want [goal] so that [benefit]" |
| **Existing Solutions** | How is this solved elsewhere? Include current workarounds and links to relevant implementations, docs, or design proposals |
| **Proposed Solution** | Technical approach and design details |
| **Alternatives Considered** | What other approaches were evaluated? |
| **Open Questions** | Unresolved technical decisions that need input (if any) |
| **Milestone Plan** | Implementation phases aligned with release milestones |

#### Proposal Labels

Use these labels to track design proposal status:

| Label | Description |
|-------|-------------|
| `Proposal/Draft` | Initial proposal, still being written |
| `Proposal/Review` | Ready for team review and feedback |
| `Proposal/Approved` | Design accepted, ready for implementation |
| `Proposal/Rejected` | Proposal declined |
| `Proposal/Implemented` | Design fully implemented |

### 3. Implementation Tracking

Once a design proposal is approved:
1. Create GitHub Issues for implementation tasks
2. Link issues back to the design proposal discussion
3. Assign issues to appropriate milestones
4. Track progress through milestone completion
