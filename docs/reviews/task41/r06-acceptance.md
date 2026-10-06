# R06 / D37 — scoped atomic publication

Scope: row 06, publication refs. No production publication or push was used.

## Contract and implementation

The release train contains one root tag and one tag for each tracked submodule.
The runner checks local and exact push-destination tag collisions before manifest
rewrites and again after final confirmation. Multiple configured push destinations
are rejected before cloning; fetch and push URLs may differ. Git validates ref
syntax. Publication sends only explicit `refs/tags/name:refs/tags/name` refspecs
with `--atomic --no-follow-tags`, disabling mirror configuration for the command.
No force, broad `--tags`, ref deletion or sequential fallback is used. Private
candidate cleanup preserves pre-existing source and remote refs.

## Regression and checks

- [Previous-commit scope regression](r06/baseline.log): behavioral FAIL on
  `5f80c9e`; both unrelated private tags were published alongside the train.
- Committed AAA fixtures cover exact train scope, source byte/index/ref
  preservation, local child-tag collision, distinct push-destination collision,
  one rejected child tag, missing atomic capability, multiple push destinations,
  and a collision created between preparation and publication.
- Parent full release package race: PASS, 123.724s. Formatting/helper extraction
  afterward did not change behavior; final root race covers the final source.
- [Root lint](r06/lint.log): zero issues.
- [Final root race](r06/root-race.log): PASS, exit 0; release package 140.751s.

## Independent final acceptance

Reviewers `r06_acceptance_a` and `r06_acceptance_b`: **100%, accepted**,
each criterion 20/20, no unresolved detected defects. Independent full release
race passes at 151.213s / 151.332s; lint has zero issues. Reviewer A independently
checks hostile mirror/followTags/configured refspecs and a competing child tag
created after preflight; atomic push preserves that child and creates no root tag.
Reviewer B independently checks counted mirror/followTags/extra refspecs and a
distinct push-destination success control. Probe setup corrections (multiline ref
comparison and hook quarantine variables) were fixture issues, with final clean
reruns retained.

Evidence: [A checks](r06/review-a-checks.log), [A probes](r06/review-a-probes.log),
[B race](r06/review-b-race.log), [B lint](r06/review-b-lint.log),
[B probes](r06/review-b-probes.log). Independent probe sources are retained as
[A source](r06/review-a-probe.go.txt) / [B source](r06/review-b-probe.go.txt).

## Limits

Preflight cannot reserve remote refs. Atomic non-forcing Git push arbitrates
conflicting concurrent updates; an identical concurrent tag can be reported
up-to-date. A lost transport response after remote acceptance leaves publication
outcome uncertain; inspect remote refs before retrying. There is no remote
rollback. Atomic capability is mandatory. These fixture checks run on macOS;
Linux live execution remains unverified. Completeness measures the five explicit
criteria, not a universal guarantee of no bugs.
