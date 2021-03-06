package controller

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/coreos-inc/alb-ingress-controller/pkg/cmd/log"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/kubernetes/pkg/apis/extensions"
)

type Rule struct {
	ingressId   *string
	SvcName     string
	CurrentRule *elbv2.Rule
	DesiredRule *elbv2.Rule
}

func NewRule(path extensions.HTTPIngressPath, ingressId *string) *Rule {
	r := &elbv2.Rule{
		Actions: []*elbv2.Action{
			&elbv2.Action{
				// TargetGroupArn: targetGroupArn,
				Type: aws.String("forward"),
			},
		},
	}

	if path.Path == "/" {
		r.IsDefault = aws.Bool(true)
		r.Priority = aws.String("default")
	} else {
		r.IsDefault = aws.Bool(false)
		r.Conditions = []*elbv2.RuleCondition{
			&elbv2.RuleCondition{
				Field:  aws.String("path-pattern"),
				Values: []*string{&path.Path},
			},
		}
	}

	rule := &Rule{
		ingressId:   ingressId,
		SvcName:     path.Backend.ServiceName,
		DesiredRule: r,
	}
	return rule
}

// SyncState compares the current and desired state of this Rule instance. Comparison
// results in no action, the creation, the deletion, or the modification of an AWS Rule to
// satisfy the ingress's current state.
func (r *Rule) SyncState(lb *LoadBalancer, l *Listener) *Rule {

	switch {
	// No DesiredState means Rule should be deleted.
	case r.DesiredRule == nil:
		log.Infof("Start Rule deletion.", *r.ingressId)
		r.delete(lb)

	// When DesiredRule is a default rule, there is nothing to be done as it was created with the
	// listener.
	case *r.DesiredRule.IsDefault:
		log.Debugf("Found desired rule that is a default and is already created with its respective listener. Rule: %s",
			*r.ingressId, log.Prettify(r.DesiredRule))
		r.CurrentRule = r.DesiredRule

	// No CurrentState means Rule doesn't exist in AWS and should be created.
	case r.CurrentRule == nil:
		log.Infof("Start Rule creation.", *r.ingressId)
		r.create(lb, l)

	// Current and Desired exist and need for modification should be evaluated.
	case r.needsModification():
		log.Infof("Start Rule modification.", *r.ingressId)
		r.modify(lb)

	default:
		log.Debugf("No listener modification required.", *r.ingressId)
	}

	return r
}

func (r *Rule) create(lb *LoadBalancer, l *Listener) error {

	createRuleInput := &elbv2.CreateRuleInput{
		Actions:     r.DesiredRule.Actions,
		Conditions:  r.DesiredRule.Conditions,
		ListenerArn: l.CurrentListener.ListenerArn,
		Priority:    aws.Int64(lb.lastRulePriority),
	}

	createRuleInput.Actions[0].TargetGroupArn = lb.TargetGroups[0].CurrentTargetGroup.TargetGroupArn

	tgIndex := lb.TargetGroups.LookupBySvc(r.SvcName)

	if tgIndex < 0 {
		log.Errorf("Failed to locate TargetGroup related to this service. Defaulting to first Target Group. SVC: %s", *r.ingressId, r.SvcName)
	} else {
		ctg := lb.TargetGroups[tgIndex].CurrentTargetGroup
		createRuleInput.Actions[0].TargetGroupArn = ctg.TargetGroupArn
	}

	createRuleOutput, err := elbv2svc.svc.CreateRule(createRuleInput)
	if err != nil {
		log.Errorf("Failed Rule creation. Rule: %s | Error: %s", *r.ingressId, log.Prettify(r.DesiredRule), err.Error())
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "CreateRule"}).Add(float64(1))
		return err
	}

	r.CurrentRule = createRuleOutput.Rules[0]

	// Increase rule priority by 1 for each creation of a rule on this listener.
	// Note: All rules must have a unique priority.
	lb.lastRulePriority += 1
	log.Errorf("Completed Rule creation. Rule: %s", *r.ingressId, log.Prettify(r.CurrentRule))
	return nil
}

func (r *Rule) modify(lb *LoadBalancer) error {
	log.Infof("Completed Rule modification. [UNIMPLEMENTED]", *r.ingressId)
	return nil
}

func (r *Rule) delete(lb *LoadBalancer) error {

	if r.CurrentRule == nil {
		log.Infof("Rule entered delete with no CurrentRule to delete. Rule: %s",
			*r.ingressId, log.Prettify(r))
		return nil
	}

	// If the current rule was a default, it's bound to the listener and won't be deleted from here.
	if *r.CurrentRule.IsDefault {
		log.Infof("Deletion hit for default rule, which is bound to the Listener. It will not be deleted from here. Rule. Rule: %s",
			*r.ingressId, log.Prettify(r))
	}

	_, err := elbv2svc.svc.DeleteRule(&elbv2.DeleteRuleInput{
		RuleArn: r.CurrentRule.RuleArn,
	})

	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "DeleteRule"}).Add(float64(1))
		return err
	}

	log.Infof("Completed Rule deletion. Rule: %s", *r.ingressId, log.Prettify(r.CurrentRule))
	return nil
}

func (r *Rule) needsModification() bool {
	cr := r.CurrentRule
	dr := r.DesiredRule

	switch {
	case cr == nil:
		return true
		// TODO: If we can populate the TargetGroupArn in NewALBIngressFromIngress, we can enable this
	// case awsutil.Prettify(cr.Actions) != awsutil.Prettify(dr.Actions):
	// 	return true
	case awsutil.Prettify(cr.Conditions) != awsutil.Prettify(dr.Conditions):
		return true
	}

	return false
}

// Equals returns true if the two CurrentRule and target rule are the same
// Does not compare priority, since this is not supported by the ingress spec
func (r *Rule) Equals(target *elbv2.Rule) bool {
	switch {
	case r.CurrentRule == nil && target == nil:
		return false
	case r.CurrentRule == nil && target != nil:
		return false
	case r.CurrentRule != nil && target == nil:
		return false
		// a rule is tightly wound to a listener which is also bound to a single TG
		// action only has 2 values, tg arn and a type, type is _always_ forward
	// case !awsutil.DeepEqual(r.CurrentRule.Actions, target.Actions):
	// 	return false
	case !awsutil.DeepEqual(r.CurrentRule.IsDefault, target.IsDefault):
		return false
	case !awsutil.DeepEqual(r.CurrentRule.Conditions, target.Conditions):
		return false
	}
	return true
}
