package forms

func ToggleStatus(props any) Node {
	return <ActionForm actionName="toggleForm">
		<input type="hidden" name="csrf_token" value={csrf.token}></input>
		<input type="hidden" name="form_id" value={data.form.id}></input>
		<input type="hidden" name="status" value={props.next}></input>
		<p class="form-error" data-gosx-field-error="status" aria-live="polite"></p>
		<p class="form-status" role="status" aria-live="polite">{action.message}</p>
		<button class={props.className} type="submit">{props.label}</button>
	</ActionForm>
}

// MoveFieldForm posts one of the two "move buttons" the FB-3 spec asks for
// in place of the old inert drag handle. It is a plain ActionForm, not an
// island: reordering is two small POSTs, matching every other builder
// action on this page, so the zero-bespoke-JavaScript invariant holds.
func MoveFieldForm(props any) Node {
	return <ActionForm class="field-move-form" actionName="moveField">
		<input type="hidden" name="csrf_token" value={csrf.token}></input>
		<input type="hidden" name="form_id" value={data.form.id}></input>
		<input type="hidden" name="field_id" value={props.id}></input>
		<input type="hidden" name="direction" value={props.direction}></input>
		<p class="form-error" data-gosx-field-error="direction" aria-live="polite"></p>
		<p class="form-status sr-only" role="status" aria-live="polite">{actions.moveField.message}</p>
		<button class="button button-compact" type="submit" aria-label={props.label} disabled={props.disabled}>{props.symbol}</button>
	</ActionForm>
}

// UpdateFieldForm edits one existing field. It never posts the field's ID:
// the ID is read-only here, so a builder edit can change how a question
// looks or behaves without ever breaking a reference (routing keys, the
// ConditionalFormatFields island's "workshop_needs" lookup) that depends on
// the ID staying stable.
func UpdateFieldForm(props any) Node {
	return <ActionForm class="field-edit-form" actionName="updateField">
		<input type="hidden" name="csrf_token" value={csrf.token}></input>
		<input type="hidden" name="form_id" value={data.form.id}></input>
		<input type="hidden" name="field_id" value={props.field.id}></input>
		<p class="form-status" role="status" aria-live="polite">{actions.updateField.message}</p>
		<div class="form-grid-two">
			<label>
				<span>Label</span>
				<input type="text" name="label" value={props.field.label} required></input>
				<p class="form-error" data-gosx-field-error="label" aria-live="polite"></p>
			</label>
			<label>
				<span>Type</span>
				<select name="type">
					<Each of={data.fieldTypes} as="option">
						<option value={option.value} selected={option.value == props.field.typeValue}>{option.label}</option>
					</Each>
				</select>
				<p class="form-error" data-gosx-field-error="type" aria-live="polite"></p>
			</label>
			<label>
				<span>Section</span>
				<select name="section">
					<Each of={data.fieldSections} as="option">
						<option value={option.value} selected={option.value == props.field.sectionValue}>{option.label}</option>
					</Each>
				</select>
				<p class="form-error" data-gosx-field-error="section" aria-live="polite"></p>
			</label>
			<label class="checkbox-control">
				<input type="checkbox" name="required" value="yes" checked={props.field.required}></input>
				<span>Required</span>
			</label>
			<label>
				<span>Placeholder</span>
				<input type="text" name="placeholder" value={props.field.placeholder}></input>
			</label>
			<label>
				<span>Help text</span>
				<input type="text" name="help" value={props.field.help}></input>
			</label>
			<label>
				<span>Options (comma-separated)</span>
				<input
					type="text"
					name="options"
					value={props.field.options}
					placeholder="Only used by a choice list field"
				></input>
				<p class="form-error" data-gosx-field-error="options" aria-live="polite"></p>
			</label>
			<label>
				<span>Max length</span>
				<input
					type="text"
					inputmode="numeric"
					name="max_length"
					value={props.field.maxLength}
					placeholder="No limit"
				></input>
				<p class="form-error" data-gosx-field-error="max_length" aria-live="polite"></p>
			</label>
		</div>
		<button class="button button-compact" type="submit">Save field</button>
	</ActionForm>
}

func RemoveFieldForm(props any) Node {
	return <ActionForm class="field-remove-form" actionName="removeField">
		<input type="hidden" name="csrf_token" value={csrf.token}></input>
		<input type="hidden" name="form_id" value={data.form.id}></input>
		<input type="hidden" name="field_id" value={props.id}></input>
		<p class="form-error" data-gosx-field-error="field" aria-live="polite"></p>
		<p class="form-status" role="status" aria-live="polite">{actions.removeField.message}</p>
		<button class="button button-compact" type="submit" aria-label={"Remove " + props.label}>Remove field</button>
	</ActionForm>
}

// AddFieldForm creates a new field. Its ID is generated server-side
// (domain.Slugify on the label, addField in page.server.go), so the builder
// never asks the organizer to invent a slug-safe, unique field ID by hand.
func AddFieldForm() Node {
	return <ActionForm class="field-add-form" actionName="addField">
		<input type="hidden" name="csrf_token" value={csrf.token}></input>
		<input type="hidden" name="form_id" value={data.form.id}></input>
		<p class="form-status" role="status" aria-live="polite">{actions.addField.message}</p>
		<div class="form-grid-two">
			<label>
				<span>Label</span>
				<input type="text" name="label" value={actions.addField.values.label} required></input>
				<p class="form-error" data-gosx-field-error="label" aria-live="polite"></p>
			</label>
			<label>
				<span>Type</span>
				<select name="type">
					<option value="">Choose a type</option>
					<Each of={data.fieldTypes} as="option">
						<option value={option.value} selected={option.value == actions.addField.values.type}>{option.label}</option>
					</Each>
				</select>
				<p class="form-error" data-gosx-field-error="type" aria-live="polite"></p>
			</label>
			<label>
				<span>Section</span>
				<select name="section">
					<option value="">Choose a section</option>
					<Each of={data.fieldSections} as="option">
						<option value={option.value} selected={option.value == actions.addField.values.section}>{option.label}</option>
					</Each>
				</select>
				<p class="form-error" data-gosx-field-error="section" aria-live="polite"></p>
			</label>
			<label class="checkbox-control">
				<input type="checkbox" name="required" value="yes" checked={actions.addField.values.required == "yes"}></input>
				<span>Required</span>
			</label>
			<label>
				<span>Placeholder</span>
				<input type="text" name="placeholder" value={actions.addField.values.placeholder}></input>
			</label>
			<label>
				<span>Help text</span>
				<input type="text" name="help" value={actions.addField.values.help}></input>
			</label>
			<label>
				<span>Options (comma-separated)</span>
				<input
					type="text"
					name="options"
					value={actions.addField.values.options}
					placeholder="Only used by a choice list field"
				></input>
				<p class="form-error" data-gosx-field-error="options" aria-live="polite"></p>
			</label>
			<label>
				<span>Max length</span>
				<input
					type="text"
					inputmode="numeric"
					name="max_length"
					value={actions.addField.values.max_length}
					placeholder="No limit"
				></input>
				<p class="form-error" data-gosx-field-error="max_length" aria-live="polite"></p>
			</label>
		</div>
		<button class="button button-primary" type="submit">Add field</button>
	</ActionForm>
}

// FormScheduleForm saves SubmissionForm.CloseAt (FB-3). Once saved,
// app/submit/page.server.go's submitProposal blocks a submission after this
// moment, so this control changes real, server-enforced behavior, not just
// display copy.
func FormScheduleForm() Node {
	return <ActionForm class="schedule-form" actionName="setFormSchedule">
		<input type="hidden" name="csrf_token" value={csrf.token}></input>
		<input type="hidden" name="form_id" value={data.form.id}></input>
		<label class="field-control">
			<span>Close at</span>
			<input type="datetime-local" name="close_at" value={data.form.closeISO}></input>
			<p class="form-error" data-gosx-field-error="close_at" aria-live="polite"></p>
		</label>
		<p class="form-status" role="status" aria-live="polite">{actions.setFormSchedule.message}</p>
		<button class="button button-compact" type="submit">Save close date</button>
	</ActionForm>
}

func Page() Node {
	return <main id="main-content" class="workspace-page" data-section={data.section}>
		<header class="workspace-header">
			<div>
				<p class="eyebrow">Intake architecture</p>
				<h1>Forms & routing</h1>
				<p>
					Build the public call, conditionally collect detail, and make ownership explainable.
				</p>
			</div>
			<div class="workspace-header-actions">
				<a class="button" href={data.form.publicURL} data-gosx-link>Preview public form</a>
				<If cond={data.form.statusValue == "open"}>
					<ToggleStatus next="closed" label="Close CFP" className="button"></ToggleStatus>
				</If>
				<If cond={data.form.statusValue == "closed"}>
					<ToggleStatus next="open" label="Open CFP" className="button button-primary"></ToggleStatus>
				</If>
			</div>
		</header>
		<p class="flash-message">{flash.notice}</p>
		<section class="form-summary-grid">
			<article class="form-summary-card summary-primary">
				<div class="summary-top">
					<span class={"status-pill status-" + data.form.statusTone}>{data.form.status}</span>
					<span class="mono">{data.form.id}</span>
				</div>
				<p class="panel-kicker">{data.form.name}</p>
				<h2>{data.form.title}</h2>
				<p>{data.form.welcomeBody}</p>
				<div class="summary-facts">
					<span>
						<strong>{data.form.fieldCount}</strong>
						fields
					</span>
					<span>
						<strong>{data.form.conditionalCount}</strong>
						condition
					</span>
					<span>
						<strong>{data.form.close}</strong>
						closes
					</span>
				</div>
			</article>
			<article class="form-summary-card">
				<p class="panel-kicker">Submission outcome</p>
				<h2>Close the loop immediately.</h2>
				<ul class="check-list">
					<li>
						<span aria-hidden="true">✓</span>
						Confirmation email is enabled
					</li>
					<li>
						<span aria-hidden="true">✓</span>
						Submitter redirects into their portal
					</li>
					<li>
						<span aria-hidden="true">✓</span>
						Category policy assigns queue, owner, and track
					</li>
				</ul>
				<p class="policy-file">
					Policy
					<code>{data.form.ruleFile}</code>
				</p>
			</article>
		</section>
		<div class="two-column-workspace">
			<section class="panel">
				<header class="panel-header">
					<div>
						<p class="panel-kicker">Builder</p>
						<h2>Question structure</h2>
					</div>
					<span>
						{data.form.fieldCount}
						fields
					</span>
				</header>
				<div class="field-builder-list">
					<Each of={data.fields} as="field">
						<article class="field-builder-row">
							<div class="field-move-controls">
								<MoveFieldForm
									id={field.id}
									direction="up"
									symbol="↑"
									label={"Move " + field.label + " up"}
									disabled={field.first}
								></MoveFieldForm>
								<MoveFieldForm
									id={field.id}
									direction="down"
									symbol="↓"
									label={"Move " + field.label + " down"}
									disabled={field.last}
								></MoveFieldForm>
							</div>
							<span class="field-index mono">{field.index}</span>
							<div>
								<strong>{field.label}</strong>
								<small>
									{field.section}
									·
									{field.kind}
								</small>
							</div>
							<span class="field-requirement">{field.requirement}</span>
							<If cond={field.locked}>
								<span class="lock-note">Locked</span>
							</If>
						</article>
						<details class="field-manage-details">
							<summary>{"Edit " + field.label}</summary>
							<UpdateFieldForm field={field}></UpdateFieldForm>
							<If cond={!field.locked}>
								<RemoveFieldForm id={field.id} label={field.label}></RemoveFieldForm>
							</If>
							<If cond={field.locked}>
								<p class="control-note">
									Locked fields power required routing and cannot be removed.
								</p>
							</If>
						</details>
					</Each>
				</div>
				<details class="field-add-details">
					<summary>Add a field</summary>
					<AddFieldForm></AddFieldForm>
				</details>
			</section>
			<aside class="stacked-panels">
				<section class="panel">
					<header class="panel-header">
						<div>
							<p class="panel-kicker">Conditional logic</p>
							<h2>Question rules</h2>
						</div>
					</header>
					<Each of={data.questionRules} as="rule">
						<article class="logic-card">
							<div>
								<span>If</span>
								<code>{rule.condition}</code>
							</div>
							<div>
								<span>Then</span>
								<code>{rule.then}</code>
							</div>
							<p>{rule.description}</p>
							<small>
								{rule.policy}
								·
								{rule.trace}
							</small>
						</article>
					</Each>
				</section>
				<section class="panel">
					<header class="panel-header">
						<div>
							<p class="panel-kicker">Guardrails</p>
							<h2>Form controls</h2>
						</div>
					</header>
					<FormScheduleForm></FormScheduleForm>
					<p class="control-note">
						The server rejects late submissions even if a stale browser still has the page open.
					</p>
				</section>
			</aside>
		</div>
		<section class="panel routing-panel">
			<header class="panel-header">
				<div>
					<p class="panel-kicker">Audited policy</p>
					<h2>Category routing table</h2>
				</div>
				<code>{data.form.ruleFile}</code>
			</header>
			<div class="data-table" role="table" aria-label="Category routing decisions">
				<div class="data-table-head routing-columns" role="row">
					<span>Category</span>
					<span>Queue</span>
					<span>Owner</span>
					<span>Track</span>
					<span>Rule</span>
				</div>
				<Each of={data.routes} as="route">
					<div class="data-table-row routing-columns" role="row">
						<strong>{route.category}</strong>
						<code>{route.queue}</code>
						<span>{route.owner}</span>
						<span>{route.track}</span>
						<span class="policy-rule">{route.rule}</span>
					</div>
				</Each>
			</div>
		</section>
	</main>
}
