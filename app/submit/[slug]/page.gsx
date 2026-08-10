package submit

func FieldError(props any) Node {
	return <p class="form-error" id={props.id + "-error"} aria-live="polite">{props.message}</p>
}

// FormFieldRow renders one schema-driven field from present.SubmissionForm's
// proposalFields or participantFields: text, textarea, select, and email
// (email reuses the input branch with inputType "email"). This is a server
// component, not an island, so the field list a builder edit produces
// appears on the next load with no client-side script involved.
func FormFieldRow(props any) Node {
	return <label class="field-row">
		<span>
			{props.field.label}
			<If cond={props.field.required}>
				<b>Required</b>
			</If>
		</span>
		<If cond={props.field.type == "textarea"}>
			<textarea
				name={props.field.id}
				maxlength={props.field.maxLength}
				placeholder={props.field.placeholder}
				aria-describedby={props.field.id + "-error"}
				required={props.field.required}
			>{props.value}</textarea>
		</If>
		<If cond={props.field.type == "select"}>
			<select
				name={props.field.id}
				aria-describedby={props.field.id + "-error"}
				required={props.field.required}
			>
				<option value="">{"Choose " + props.field.label}</option>
				<Each of={props.field.options} as="option">
					<option value={option.value} selected={option.value == props.value}>{option.label}</option>
				</Each>
			</select>
		</If>
		<If cond={props.field.type != "textarea" && props.field.type != "select"}>
			<input
				type={props.field.inputType}
				name={props.field.id}
				value={props.value}
				maxlength={props.field.maxLength}
				placeholder={props.field.placeholder}
				aria-describedby={props.field.id + "-error"}
				required={props.field.required}
			></input>
		</If>
		<If cond={props.field.help != ""}>
			<small>{props.field.help}</small>
		</If>
		<FieldError id={props.field.id} message={props.error}></FieldError>
	</label>
}

//gosx:island
func ConditionalFormatFields(props any) Node {
	format := signal.New(props.formatValue)
	selectFormat := func() { format.Set(value) }
	return <div class="conditional-format-fields">
		<label>
			<span>
				Format
				<b>Required</b>
			</span>
			<select name="format" aria-describedby="format-error" required onChange={selectFormat}>
				<option value="">Choose format</option>
				<Each of={props.formats} as="option">
					<option value={option.value} selected={option.value == format.Get()}>{option.label}</option>
				</Each>
			</select>
			<p class="form-error" id="format-error" aria-live="polite">{props.formatError}</p>
		</label>
		<label class="conditional-field" hidden={format.Get() != "Workshop"}>
			<span>
				Workshop logistics
				<b>Required for workshops</b>
			</span>
			<textarea
				name="workshop_needs"
				maxlength="800"
				placeholder="Room setup, facilitation, materials, and participant limit"
				aria-describedby="workshop_needs-error"
				required={format.Get() == "Workshop"}
				disabled={format.Get() != "Workshop"}
			>{props.workshopNeeds}</textarea>
			<small>
				Conditional CFP policy ·
				{props.conditionalWhy}
			</small>
			<p class="form-error" id="workshop_needs-error" aria-live="polite">{props.workshopError}</p>
		</label>
	</div>
}

func Page() Node {
	return <main id="main-content" class="submission-flow">
		<aside class="submission-context">
			<p class="eyebrow">{data.event.name}</p>
			<h1>{data.form.title}</h1>
			<p>{data.form.heading}</p>
			<dl>
				<div>
					<dt>Event</dt>
					<dd>{data.event.dates}</dd>
				</div>
				<div>
					<dt>Location</dt>
					<dd>{data.event.location}</dd>
				</div>
				<div>
					<dt>Deadline</dt>
					<dd>{data.form.close}</dd>
				</div>
			</dl>
			<div class="submission-assurances">
				<span>
					<i aria-hidden="true">✓</i>
					Confirmation email
				</span>
				<span>
					<i aria-hidden="true">✓</i>
					Immediate speaker portal
				</span>
				<span>
					<i aria-hidden="true">✓</i>
					Two-round review
				</span>
			</div>
		</aside>
		<section class="submission-form-panel">
			<header>
				<p class="panel-kicker">2026 call for speakers</p>
				<h2>Tell us what you have learned.</h2>
				<p>{data.form.body}</p>
			</header>
			<If cond={!data.form.open}>
				<div class="closed-notice">
					<strong>This call is closed.</strong>
					<p>
						Contact the program team if you believe this is an error.
					</p>
				</div>
			</If>
			<If cond={data.form.open}>
				<ActionForm class="public-form" actionName="submitProposal" aria-describedby="submission-form-status">
					<input type="hidden" name="csrf_token" value={csrf.token}></input>
					<input type="hidden" name="form_id" value={data.form.id}></input>
					<p class="form-status" id="submission-form-status" role="alert" tabindex="-1">{action.message}</p>
					<fieldset>
						<legend>
							<span>01</span>
							Proposal
						</legend>
						<Each of={data.proposalFields} as="field">
							<If cond={field.id == "format"}>
								<ConditionalFormatFields
									formats={data.formats}
									formatValue={actions.submitProposal.values.format}
									formatError={actions.submitProposal.fieldErrors.format}
									workshopNeeds={actions.submitProposal.values.workshop_needs}
									workshopError={actions.submitProposal.fieldErrors.workshop_needs}
									conditionalWhy={data.form.conditionalWhy}
								></ConditionalFormatFields>
							</If>
							<If cond={field.id != "format"}>
								<FormFieldRow
									field={field}
									value={actions.submitProposal.values[field.id]}
									error={actions.submitProposal.fieldErrors[field.id]}
								></FormFieldRow>
							</If>
						</Each>
					</fieldset>
					<fieldset>
						<legend>
							<span>02</span>
							Participant
						</legend>
						<Each of={data.participantFields} as="field">
							<FormFieldRow
								field={field}
								value={actions.submitProposal.values[field.id]}
								error={actions.submitProposal.fieldErrors[field.id]}
							></FormFieldRow>
						</Each>
					</fieldset>
					<footer>
						<p>
							By submitting, you agree to the event code of conduct. Payments are not collected.
						</p>
						<button class="button button-primary" type="submit">
							Submit proposal
							<span aria-hidden="true">→</span>
						</button>
					</footer>
				</ActionForm>
			</If>
		</section>
	</main>
}
