# costgate and Infracost

> Checked on **2026-11-04** against the Infracost documentation, pricing page and repositories
> linked in each row; Infracost text is paraphrased. Infracost changes often: if a row is out of
> date, open an issue with the source.

[Infracost](https://www.infracost.io) shows cloud cost estimates in pull requests for AWS, Azure
and Google. costgate solves a narrower problem: an offline, fail-closed cost gate for AWS. This page
lists what both do, where they differ, and when each one fits better. How costgate works inside:
[how-it-works.md](how-it-works.md).

Versions compared:

| Tool | Version | License |
|---|---|---|
| costgate | 0.2.0 | MIT |
| Infracost CLI 2.x ([`infracost/cli`](https://github.com/infracost/cli)) | v2.18.3, released 2026-10-28 | Apache-2.0 |
| Infracost Cloud and its plans | [pricing page](https://www.infracost.io/pricing), undated | commercial |

The 0.10.x CLI ([`infracost/infracost`](https://github.com/infracost/infracost)) is end-of-life:
its [`AGENTS.md`](https://github.com/infracost/infracost/blob/master/AGENTS.md) says development
moved to `infracost/cli` and related repositories with Infracost 2.0. This page compares against
2.x.

## What both do

| Capability | costgate | Infracost |
|---|---|---|
| Read a Terraform or OpenTofu plan JSON (`show -json`) | yes; the only input | yes, `infracost scan plan.json` ([plan JSON](https://www.infracost.io/docs/features/terraform_plan_json/)) |
| Monthly cost before and after the change, per resource | yes | yes |
| Hourly resources priced at 730 hours per month | yes, overridable per resource | yes ([usage-based resources](https://www.infracost.io/docs/features/usage_based_resources/)) |
| Usage-based costs from a file in the repository | `usage.json` (JSON), per type and per resource | `infracost-usage.yml` (YAML), per type and per resource, with `[*]` for `count` and `for_each` ([usage-based resources](https://www.infracost.io/docs/features/usage_based_resources/)) |
| PR comment updated in place, not duplicated | Markdown with a `<!-- costgate -->` marker; your job posts or updates it ([integration](integration/github-actions.md)) | `infracost-ci diff` posts or updates it ([GitHub Actions](https://www.infracost.io/docs/integrations/github_actions/)) |
| Machine-readable output | JSON report and JSON Lines | JSON ([CLI commands](https://www.infracost.io/docs/features/cli_commands/)) |
| No cloud credential needed | yes | yes ([FAQ](https://www.infracost.io/docs/faq/)) |
| Public on-demand prices by default | yes | yes; negotiated prices are a paid feature (below) |
| Open-source CLI | yes | yes |

## Where they differ

| Topic | costgate | Infracost |
|---|---|---|
| Account | none | log in before any command (`infracost auth login`); CI uses a token in `INFRACOST_CLI_AUTHENTICATION_TOKEN` ([CLI README](https://github.com/infracost/cli#login)) |
| Network while estimating | none: the prices are inside the binary | queries `pricing.api.infracost.io`; the request carries the cost-related parameters, a count of resource types and error-tracking context, not the plan JSON or credentials ([FAQ](https://www.infracost.io/docs/faq/)). In CI, `infracost-ci diff` also uploads the run (estimate and pull request metadata) to Infracost Cloud ([GitHub Actions](https://www.infracost.io/docs/integrations/github_actions/)) |
| Where prices come from | snapshot of the public AWS Price List Bulk API, versioned in this repository with a `sha256` and changed only by a reviewed pull request | Infracost's Cloud Pricing API, queried at run time; custom price books are applied there ([FAQ](https://www.infracost.io/docs/faq/)) |
| Code downloaded at run time | none; one static binary with checksums signed by cosign keyless | parser and provider plugins downloaded on use, latest version unless pinned by environment variable ([CLI README](https://github.com/infracost/cli#plugins)) |
| Input formats | plan JSON | Terraform HCL without a plan (the default), plan JSON, Terragrunt, CloudFormation and CDK ([plan JSON](https://www.infracost.io/docs/features/terraform_plan_json/), [CLI README](https://github.com/infracost/cli), [pricing](https://www.infracost.io/pricing)) |
| Estimates on a developer machine before a plan exists | no: needs a plan | yes, from HCL, plus editor and AI agent integrations ([`infracost/lsp`](https://github.com/infracost/lsp), [`infracost/agent-skills`](https://github.com/infracost/agent-skills)) |
| Clouds and coverage | AWS only: 20 resource types with a formula plus a list of types without a charge, priced from a us-east-1 snapshot ([covered resources](../README.md#covered-resources-us-east-1-snapshot-of-2026-11-03)) | AWS, Azure and Google, over 1,100 Terraform resources ([supported resources](https://www.infracost.io/docs/supported_resources/overview/)) |
| Resource the tool cannot price | **fail-closed**: a created, updated or replaced one makes the run exit 2 | unsupported types are reported in the `scan` output ([supported resources](https://www.infracost.io/docs/supported_resources/overview/)); no CLI option to fail on them was found in the docs |
| Usage not declared | counts as 0 and is listed in the report as a missing premise | predefined usage values that aim at about US$ 5 per month for common configurations, from the Infracost Cloud usage defaults (a paid feature) or from the template usage file ([usage-based resources](https://www.infracost.io/docs/features/usage_based_resources/)) |
| Auto Scaling groups | not priced yet (exit 2 when created or changed) | priced from `desired_capacity`, else `min_size`, else 1 instance ([FAQ](https://www.infracost.io/docs/faq/)) |
| Cost gate | in the CLI: exit 1 above `--max-monthly-delta` or `--max-monthly-total`, exit 2 for unpriced resources; your CI decides how to accept them (for example a `cost-approved` label) | guardrails in Infracost Cloud, on the Cloud plan: cost change, percentage change or monthly total; they notify, comment or block through a required status check, and a reviewer unblocks in the dashboard. When a guardrail blocks, `infracost-ci diff` exits 1 ([guardrails](https://www.infracost.io/docs/infracost_cloud/guardrails/), [GitHub Actions](https://www.infracost.io/docs/integrations/github_actions/), [pricing](https://www.infracost.io/pricing)) |
| Same result on a later run | yes for the same costgate version: the snapshot travels with the binary, and the output does not depend on the clock or the machine | uses the prices the API returns at the time of the run |
| Keys of the AWS bill on each cost line | SKU, usage type with region prefix, rate code, invoice description and billing rule, matching CUR 2.0 columns | not found in the docs on 2026-11-04 |
| Output for SQL engines | JSON Lines, one standalone document per cost line | not found in the docs |
| Policies | none | FinOps best-practice and tagging policies, on the Cloud plan ([pricing](https://www.infracost.io/pricing)) |
| Dashboards, notifications, workflow | none | dashboards, issue explorer, audit trail and email, Slack or Microsoft Teams notifications, on the Cloud plan ([pricing](https://www.infracost.io/pricing), [guardrails](https://www.infracost.io/docs/infracost_cloud/guardrails/)) |
| Source control apps | none: runs as a step in your CI | apps for GitHub, GitLab and Azure Repos with no pipeline change; Bitbucket through Pipelines ([plan JSON](https://www.infracost.io/docs/features/terraform_plan_json/)) |
| Currency and discounts | USD list prices | other currencies; AWS EDP, Azure EA and custom price books ([FAQ](https://www.infracost.io/docs/faq/)); custom price books are on the Cloud plan, SKU-level overrides on Enterprise ([pricing](https://www.infracost.io/pricing)) |
| Vendor and support | open-source project; support through GitHub issues | company certified SOC 2 Type II ([FAQ](https://www.infracost.io/docs/faq/)); email support from the Starter plan ([pricing](https://www.infracost.io/pricing)) |
| Price | free | CI/CD plan free up to 1,000 runs per month; Starter US$ 250/month (10,000 runs); Cloud US$ 1,000/month with 10 admin seats, developer seats extra; Enterprise on request ([pricing](https://www.infracost.io/pricing)) |

## When each fits better

| costgate fits when | Infracost fits when |
|---|---|
| the infrastructure is on AWS | you run on more than one cloud, or need its wider resource coverage |
| the cost job must run without an account, a token or network egress | you want estimates in the editor, from HCL, before a plan |
| a resource that cannot be priced must fail the check | you want policies, dashboards and notifications for a FinOps team |
| prices must be auditable in git and the output reproducible | you price with negotiated discounts or in another currency |
| estimates must be matched against the AWS bill by SKU and rate code | you prefer a managed service with support and apps for your source control |

The two can run side by side: Infracost for broad visibility, costgate as the fail-closed AWS gate
of the same pull request.

## Sources

All checked on 2026-11-04:

- Infracost CLI: [README](https://github.com/infracost/cli) (login, plugins),
  [releases](https://github.com/infracost/cli/releases) (v2.18.3, 2026-10-28)
- Infracost 0.10.x: [`AGENTS.md`](https://github.com/infracost/infracost/blob/master/AGENTS.md) (end of life)
- Infracost docs: [FAQ](https://www.infracost.io/docs/faq/),
  [CLI commands](https://www.infracost.io/docs/features/cli_commands/),
  [plan JSON](https://www.infracost.io/docs/features/terraform_plan_json/),
  [usage-based resources](https://www.infracost.io/docs/features/usage_based_resources/),
  [supported resources](https://www.infracost.io/docs/supported_resources/overview/),
  [guardrails](https://www.infracost.io/docs/infracost_cloud/guardrails/),
  [GitHub Actions](https://www.infracost.io/docs/integrations/github_actions/)
- Infracost [pricing](https://www.infracost.io/pricing) (no date on the page)
- costgate: [README](../README.md), [how it works](how-it-works.md),
  [ADR-001](adr/001-go-snapshot-bulk-api-e-fail-closed.md)
