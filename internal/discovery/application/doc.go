// Package application is the discovery context's use-case layer.
//
// It is split into two halves:
//
//   - command/... : one package per intent that changes state or triggers a
//     side effect. Every handler follows load -> domain method -> save.
//   - query/...   : read models. They never mutate.
//
// The pipeline for one keyword, as triggered by the operator one button at a
// time:
//
//	startexploration   plant root seeds
//	      |
//	      v
//	expandkeyword      fetch related queries, add children (depth <= 2)
//	      |
//	      v
//	recordobservation  store the 30D/90D/12M/5Y series
//	      |
//	      v
//	judgetrend         cross-check windows, classify the trend (BR-01)
//	      |
//	      v
//	promote            gate -> SeedCandidate, eligible for another expand
//
// Branches:
//
//   - marktoobroad closes a word too vague to carry intent (BR-04).
//   - openverification is operator recovery when the provider asks for a human.
//   - query/dashboard is the read model behind the console.
//
// This stage is a manual tool, so the order lives in the operator's clicks and
// in flow_test.go. When rounds become automatic, add a pipeline here; do not
// duplicate the ordering inside adapters.
package application
