// Copyright 2021 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tracing

import "fmt"

// PlanTrace indicates for the Plan trace information
type PlanTrace struct {
	ID         int          `json:"id"`
	TP         string       `json:"type"`
	Children   []*PlanTrace `json:"-"`
	ChildrenID []int        `json:"children"`
	Cost       float64      `json:"cost"`
	Selected   bool         `json:"selected"`
	ProperType string       `json:"property"`
	// ExplainInfo should be implemented by each implemented Plan
	ExplainInfo string `json:"info"`
}

// LogicalOptimizeTracer indicates the trace for the whole logicalOptimize processing
type LogicalOptimizeTracer struct {
	FinalLogicalPlan []*PlanTrace                 `json:"final"`
	Steps            []*LogicalRuleOptimizeTracer `json:"steps"`
	// curRuleTracer indicates the current rule Tracer during optimize by rule
	curRuleTracer *LogicalRuleOptimizeTracer
}

// AppendRuleTracerBeforeRuleOptimize add plan tracer before optimize
func (tracer *LogicalOptimizeTracer) AppendRuleTracerBeforeRuleOptimize(index int, name string, before *PlanTrace) {
	ruleTracer := buildLogicalRuleOptimizeTracerBeforeOptimize(index, name, before)
	tracer.Steps = append(tracer.Steps, ruleTracer)
	tracer.curRuleTracer = ruleTracer
}

// AppendRuleTracerStepToCurrent add rule optimize step to current
func (tracer *LogicalOptimizeTracer) AppendRuleTracerStepToCurrent(id int, tp, reason, action string) {
	index := len(tracer.curRuleTracer.Steps)
	tracer.curRuleTracer.Steps = append(tracer.curRuleTracer.Steps, LogicalRuleOptimizeTraceStep{
		ID:     id,
		TP:     tp,
		Reason: reason,
		Action: action,
		Index:  index,
	})
}

// RecordFinalLogicalPlan add plan trace after logical optimize
func (tracer *LogicalOptimizeTracer) RecordFinalLogicalPlan(final *PlanTrace) {
	tracer.FinalLogicalPlan = toFlattenPlanTrace(final)
	tracer.removeUselessStep()
}

func (tracer *LogicalOptimizeTracer) removeUselessStep() {
	newSteps := make([]*LogicalRuleOptimizeTracer, 0)
	for _, step := range tracer.Steps {
		if len(step.Steps) > 0 {
			newSteps = append(newSteps, step)
		}
	}
	tracer.Steps = newSteps
}

// LogicalRuleOptimizeTracer indicates the trace for the LogicalPlan tree before and after
// logical rule optimize
type LogicalRuleOptimizeTracer struct {
	Index    int                            `json:"index"`
	Before   []*PlanTrace                   `json:"before"`
	RuleName string                         `json:"name"`
	Steps    []LogicalRuleOptimizeTraceStep `json:"steps"`
}

// buildLogicalRuleOptimizeTracerBeforeOptimize build rule tracer before rule optimize
func buildLogicalRuleOptimizeTracerBeforeOptimize(index int, name string, before *PlanTrace) *LogicalRuleOptimizeTracer {
	return &LogicalRuleOptimizeTracer{
		Index:    index,
		Before:   toFlattenPlanTrace(before),
		RuleName: name,
		Steps:    make([]LogicalRuleOptimizeTraceStep, 0),
	}
}

// LogicalRuleOptimizeTraceStep indicates the trace for the detailed optimize changing in
// logical rule optimize
type LogicalRuleOptimizeTraceStep struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
	ID     int    `json:"id"`
	TP     string `json:"type"`
	Index  int    `json:"index"`
}

// toFlattenPlanTrace transform plan into PlanTrace
func toFlattenPlanTrace(root *PlanTrace) []*PlanTrace {
	wrapper := &flattenWrapper{flatten: make([]*PlanTrace, 0)}
	flattenLogicalPlanTrace(root, wrapper)
	return wrapper.flatten
}

type flattenWrapper struct {
	flatten []*PlanTrace
}

func flattenLogicalPlanTrace(node *PlanTrace, wrapper *flattenWrapper) {
	newNode := &PlanTrace{
		ID:          node.ID,
		TP:          node.TP,
		ChildrenID:  make([]int, 0),
		Cost:        node.Cost,
		ExplainInfo: node.ExplainInfo,
	}
	if len(node.Children) < 1 {
		wrapper.flatten = append(wrapper.flatten, newNode)
		return
	}
	for _, child := range node.Children {
		newNode.ChildrenID = append(newNode.ChildrenID, child.ID)
	}
	for _, child := range node.Children {
		flattenLogicalPlanTrace(child, wrapper)
	}
	wrapper.flatten = append(wrapper.flatten, newNode)
}

// CETraceRecord records an expression and related cardinality estimation result.
type CETraceRecord struct {
	TableID   int64  `json:"-"`
	TableName string `json:"table_name"`
	Type      string `json:"type"`
	Expr      string `json:"expr"`
	RowCount  uint64 `json:"row_count"`
}

// DedupCETrace deduplicate a slice of *CETraceRecord and return the deduplicated slice
func DedupCETrace(records []*CETraceRecord) []*CETraceRecord {
	ret := make([]*CETraceRecord, 0, len(records))
	exists := make(map[CETraceRecord]struct{}, len(records))
	for _, rec := range records {
		if _, ok := exists[*rec]; !ok {
			ret = append(ret, rec)
			exists[*rec] = struct{}{}
		}
	}
	return ret
}

// PhysicalOptimizeTracer indicates the trace for the whole physicalOptimize processing
type PhysicalOptimizeTracer struct {
	PhysicalPlanCostDetails map[int]*PhysicalPlanCostDetail `json:"costs"`
	// final indicates the final physical plan trace
	Final      []*PlanTrace                `json:"final"`
	Candidates map[int]*CandidatePlanTrace `json:"candidates"`
}

// AppendCandidate appends physical CandidatePlanTrace in tracer.
// If the candidate already exists, the previous candidate would be covered depends on whether it has mapping logical plan
func (tracer *PhysicalOptimizeTracer) AppendCandidate(c *CandidatePlanTrace) {
	old, exists := tracer.Candidates[c.ID]
	if exists && len(old.MappingLogicalPlan) > 0 && len(c.MappingLogicalPlan) < 1 {
		return
	}
	tracer.Candidates[c.ID] = c
}

// RecordFinalPlanTrace records final physical plan trace
func (tracer *PhysicalOptimizeTracer) RecordFinalPlanTrace(root *PlanTrace) {
	tracer.Final = toFlattenPlanTrace(root)
	tracer.buildCandidatesInfo()
}

// CandidatePlanTrace indicates info for candidate
type CandidatePlanTrace struct {
	*PlanTrace
	MappingLogicalPlan string `json:"mapping"`
}

// buildCandidatesInfo builds candidates info
func (tracer *PhysicalOptimizeTracer) buildCandidatesInfo() {
	if tracer == nil || len(tracer.Candidates) < 1 {
		return
	}
	fID := make(map[int]struct{}, len(tracer.Final))
	for _, plan := range tracer.Final {
		fID[plan.ID] = struct{}{}
	}

	for _, candidate := range tracer.Candidates {
		if _, ok := fID[candidate.ID]; ok {
			candidate.Selected = true
		}
	}
}

// CodecPlanName returns tp_id of plan.
func CodecPlanName(tp string, id int) string {
	return fmt.Sprintf("%v_%v", tp, id)
}

// OptimizeTracer indicates tracer for optimizer
type OptimizeTracer struct {
	// Logical indicates logical plan
	Logical *LogicalOptimizeTracer `json:"logical"`
	// Physical indicates physical plan
	Physical *PhysicalOptimizeTracer `json:"physical"`
	// FinalPlan indicates the plan after post optimize
	FinalPlan []*PlanTrace `json:"final"`
	// IsFastPlan indicates whether the plan is generated by fast plan
	IsFastPlan bool `json:"isFastPlan"`
}

// SetFastPlan sets fast plan
func (tracer *OptimizeTracer) SetFastPlan(final *PlanTrace) {
	tracer.FinalPlan = toFlattenPlanTrace(final)
	tracer.IsFastPlan = true
}

// RecordFinalPlan records plan after post optimize
func (tracer *OptimizeTracer) RecordFinalPlan(final *PlanTrace) {
	tracer.FinalPlan = toFlattenPlanTrace(final)
}

// PhysicalPlanCostDetail indicates cost detail
type PhysicalPlanCostDetail struct {
	id     int
	tp     string
	params map[string]interface{}
	desc   string
}

// NewPhysicalPlanCostDetail creates a cost detail
func NewPhysicalPlanCostDetail(id int, tp string) *PhysicalPlanCostDetail {
	return &PhysicalPlanCostDetail{
		id:     id,
		tp:     tp,
		params: make(map[string]interface{}),
	}
}

// AddParam adds param
func (d *PhysicalPlanCostDetail) AddParam(k string, v interface{}) *PhysicalPlanCostDetail {
	d.params[k] = v
	return d
}

// SetDesc sets desc
func (d *PhysicalPlanCostDetail) SetDesc(desc string) {
	d.desc = desc
}

// GetPlanID gets plan id
func (d *PhysicalPlanCostDetail) GetPlanID() int {
	return d.id
}

// GetPlanType gets plan type
func (d *PhysicalPlanCostDetail) GetPlanType() string {
	return d.tp
}

// Exists checks whether key exists in params
func (d *PhysicalPlanCostDetail) Exists(k string) bool {
	_, ok := d.params[k]
	return ok
}
