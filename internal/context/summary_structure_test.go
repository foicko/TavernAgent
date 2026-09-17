package context

import "testing"

func TestSummaryRequiresRealStructuredSections(t *testing.T) {
	for name, xml := range map[string]string{
		"comment_only":           "<story_checkpoint><!-- <narrative_arc>fake</narrative_arc><open_loops>fake</open_loops> --></story_checkpoint>",
		"nested_root":            "<story_checkpoint><story_checkpoint><narrative_arc>fake</narrative_arc><open_loops>fake</open_loops></story_checkpoint></story_checkpoint>",
		"duplicate_section":      "<story_checkpoint><narrative_arc>first</narrative_arc><narrative_arc>second</narrative_arc><open_loops>none</open_loops></story_checkpoint>",
		"empty_narrative":        "<story_checkpoint><narrative_arc>  </narrative_arc><open_loops>none</open_loops></story_checkpoint>",
		"wrong_parent":           "<story_checkpoint><character_dynamics><narrative_arc>fake</narrative_arc></character_dynamics><open_loops>none</open_loops></story_checkpoint>",
		"processing_instruction": "<story_checkpoint><?tool execute?><narrative_arc>story</narrative_arc><open_loops>none</open_loops></story_checkpoint>",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseCompactionSummary(xml); err == nil {
				t.Fatal("invalid summary was accepted and could replace source history")
			}
		})
	}
	for _, xml := range []string{
		"<story_checkpoint><narrative_arc>They reached the inn.</narrative_arc><open_loops/></story_checkpoint>",
		"<story_checkpoint><narrative_arc><![CDATA[They reached <the inn>.]]></narrative_arc><character_dynamics><mindset character=\"Guide\">Calm.</mindset></character_dynamics><open_loops>Return tomorrow.</open_loops></story_checkpoint>",
	} {
		if _, err := ParseCompactionSummary(xml); err != nil {
			t.Errorf("valid summary rejected: %v", err)
		}
	}
}
