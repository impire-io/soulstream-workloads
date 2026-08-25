package wrap

import (
	"fmt"
	"strings"
	"time"

	"github.com/impire-io/soulstream-core/realm"

	"github.com/impire-io/soulstream-workloads/declaration"
)

// DeclaredConfig maps an agent declaration onto an engine config: the wake
// entries become the wake set, the declaration's topic becomes the home
// topic, the budget block becomes the 0006 knobs, and the instructions
// reference becomes the per-wake record source (client is the engine's own
// connection — the runtime-side-reads decision). base carries everything the
// declaration does not say (template, scratch, timeouts); its persona must
// be the declaration's persona, because the connection's credential is the
// engine's whole authority and a declaration cannot borrow another's.
//
// A declared budget of all zeros is the operator saying "unbudgeted" —
// mapped to the explicit opt-out, logged once at startup, never silently
// re-defaulted. When the declaration carries instructions and base's prompt
// lacks {{INSTRUCTIONS}}, the placeholder is appended ("\n\n{{INSTRUCTIONS}}")
// so presets deliver them without template authoring — visible,
// deterministic, and a no-op for undeclared runs.
func DeclaredConfig(base Config, d declaration.Declaration, client *realm.Client) (Config, error) {
	if err := d.Validate(); err != nil {
		return Config{}, fmt.Errorf("wrap: declaration: %w", err)
	}
	if d.Role != declaration.RoleAgent {
		return Config{}, fmt.Errorf("wrap: only agent declarations drive the wake engine (role is %q)", d.Role)
	}
	if base.Persona == "" {
		base.Persona = d.Persona
	}
	if d.Persona != base.Persona {
		return Config{}, fmt.Errorf("wrap: declaration persona %q does not match the credential's persona %q — the connection is the authority",
			d.Persona, base.Persona)
	}
	base.HomeTopic = d.Topic

	set := &WakeSet{}
	for _, e := range d.Wake {
		switch e.Kind {
		case declaration.WakeMention:
			set.Mention = true
		case declaration.WakeTopic:
			set.Topics = append(set.Topics, TopicWake{Path: e.Path, Types: e.EffectiveTypes()})
		case declaration.WakeSchedule:
			var ttl time.Duration
			if e.TTL != "" {
				parsed, err := time.ParseDuration(e.TTL)
				if err != nil {
					return Config{}, fmt.Errorf("wrap: schedule %q ttl: %w", e.Name, err)
				}
				ttl = parsed
			}
			set.Schedules = append(set.Schedules, ScheduleWake{Name: e.Name, Pattern: e.Pattern, TTL: ttl})
		case declaration.WakeSubject:
			set.Subjects = append(set.Subjects, SubjectWake{Subject: e.Subject})
		default:
			return Config{}, fmt.Errorf("wrap: wake kind %q is not runnable", e.Kind)
		}
	}
	if err := set.Validate(base.HomeTopic); err != nil {
		return Config{}, err
	}
	base.Wakes = set

	if d.Budget != nil {
		b := Budget{MaxHops: d.Budget.MaxHops}
		if d.Budget.Window != nil && d.Budget.Window.Max > 0 {
			per, err := time.ParseDuration(d.Budget.Window.Per)
			if err != nil {
				return Config{}, fmt.Errorf("wrap: budget window.per: %w", err)
			}
			b.WindowMax = d.Budget.Window.Max
			b.WindowPer = per
		}
		base.Budget = b
		if b == (Budget{}) {
			// An explicit all-zero budget block is the unbudgeted standing,
			// stated by the operator — not a gap for defaults to fill.
			base.Unbudgeted = true
		}
	}

	if d.Instructions != nil {
		if client == nil {
			return Config{}, fmt.Errorf("wrap: declared instructions need the engine's realm client")
		}
		base.Instructions = NewRecordInstructions(client, d.Instructions.Topic, d.Instructions.Artefact)
		if base.Template.Prompt != "" && !strings.Contains(base.Template.Prompt, "{{INSTRUCTIONS}}") {
			base.Template.Prompt += "\n\n{{INSTRUCTIONS}}"
		}
	}

	return base, nil
}
