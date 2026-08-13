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
		<If cond={props.field.isTextarea}>
			<textarea
				name={props.field.id}
				maxlength={props.field.maxLength}
				placeholder={props.field.placeholder}
				aria-describedby={props.field.id + "-error"}
				required={props.field.required}
			>{props.value}</textarea>
		</If>
		<If cond={props.field.isSelect}>
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
		<If cond={props.field.isInput}>
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
	answer := signal.New(props.sourceValue)
	setAnswer := func() { answer.Set(value) }
	return <div class="conditional-format-fields">
		<label class="field-row">
			<span>
				{props.source.label}
				<If cond={props.source.required}>
					<b>Required</b>
				</If>
			</span>
			<If cond={props.source.isTextarea}>
				<textarea
					name={props.source.id}
					maxlength={props.source.maxLength}
					placeholder={props.source.placeholder}
					aria-describedby={props.source.id + "-error"}
					required={props.source.required}
					onInput={setAnswer}
				>{props.sourceValue}</textarea>
			</If>
			<If cond={props.source.isSelect}>
				<select
					name={props.source.id}
					aria-describedby={props.source.id + "-error"}
					required={props.source.required}
					onChange={setAnswer}
				>
					<option value="">{"Choose " + props.source.label}</option>
					<Each of={props.source.options} as="option">
						<option value={option.value} selected={option.value == answer.Get()}>{option.label}</option>
					</Each>
				</select>
			</If>
			<If cond={props.source.isInput}>
				<input
					type={props.source.inputType}
					name={props.source.id}
					value={props.sourceValue}
					maxlength={props.source.maxLength}
					placeholder={props.source.placeholder}
					aria-describedby={props.source.id + "-error"}
					required={props.source.required}
					onInput={setAnswer}
				></input>
			</If>
			<If cond={props.source.help != ""}>
				<small>{props.source.help}</small>
			</If>
			<p
				class="form-error"
				id={props.source.id + "-error"}
				data-gosx-field-error={props.source.id}
				aria-live="polite"
			>{props.sourceError}</p>
		</label>
		<Each of={props.source.targets} as="target">
			<label class="field-row conditional-field" hidden={answer.Get() != target.ruleValue}>
				<span>
					{target.label}
					<If cond={target.required}>
						<b>Required when shown</b>
					</If>
				</span>
				<If cond={target.isTextarea}>
					<textarea
						name={target.id}
						maxlength={target.maxLength}
						placeholder={target.placeholder}
						aria-describedby={target.id + "-error"}
						required={target.required && answer.Get() == target.ruleValue}
						disabled={answer.Get() != target.ruleValue}
					>{target.value}</textarea>
				</If>
				<If cond={target.isSelect}>
					<select
						name={target.id}
						aria-describedby={target.id + "-error"}
						required={target.required && answer.Get() == target.ruleValue}
						disabled={answer.Get() != target.ruleValue}
					>
						<option value="">{"Choose " + target.label}</option>
						<Each of={target.options} as="option">
							<option value={option.value} selected={option.value == target.value}>{option.label}</option>
						</Each>
					</select>
				</If>
				<If cond={target.isInput}>
					<input
						type={target.inputType}
						name={target.id}
						value={target.value}
						maxlength={target.maxLength}
						placeholder={target.placeholder}
						aria-describedby={target.id + "-error"}
						required={target.required && answer.Get() == target.ruleValue}
						disabled={answer.Get() != target.ruleValue}
					></input>
				</If>
				<If cond={target.help != ""}>
					<small>{target.help}</small>
				</If>
				<small>
					Conditional CFP policy ·
					{target.conditionalWhy}
				</small>
				<p
					class="form-error"
					id={target.id + "-error"}
					data-gosx-field-error={target.id}
					aria-live="polite"
				></p>
			</label>
		</Each>
	</div>
}

//gosx:island
func PreviewSubmissionForm(props any) Node {
	status := signal.New("")
	saveDraft := func() {
		status.Set("Demo only: your draft is staged in this walkthrough. Nothing was sent or saved.")
	}
	submitProposal := func() {
		status.Set("Demo only: your proposal is ready for review. Nothing was sent or saved.")
	}
	return <form
		class="public-form preview-form-interactive"
		method="get"
		action=""
		onSubmit={submitProposal}
		aria-describedby="preview-submission-status"
		data-preview-only="true"
	>
		<p class="form-status" id="preview-submission-status" role="status" aria-live="polite">{status.Get()}</p>
		<fieldset>
			<legend>
				<span>01</span>
				Proposal
			</legend>
			<Each of={props.proposalFields} as="field">
				<label class="field-row">
					<span>
						{field.label}
						<If cond={field.required}>
							<b>Required</b>
						</If>
					</span>
					<If cond={field.isTextarea}>
						<textarea
							name={field.id}
							maxlength={field.maxLength}
							placeholder={field.placeholder}
							required={field.required}
						></textarea>
					</If>
					<If cond={field.isSelect}>
						<select name={field.id} required={field.required}>
							<option value="">{"Choose " + field.label}</option>
							<Each of={field.options} as="option">
								<option value={option.value}>{option.label}</option>
							</Each>
						</select>
					</If>
					<If cond={field.isInput}>
						<input
							type={field.inputType}
							name={field.id}
							maxlength={field.maxLength}
							placeholder={field.placeholder}
							required={field.required}
						></input>
					</If>
					<If cond={field.help != ""}>
						<small>{field.help}</small>
					</If>
				</label>
			</Each>
		</fieldset>
		<fieldset>
			<legend>
				<span>02</span>
				Participant
			</legend>
			<Each of={props.participantFields} as="field">
				<label class="field-row">
					<span>
						{field.label}
						<If cond={field.required}>
							<b>Required</b>
						</If>
					</span>
					<If cond={field.isTextarea}>
						<textarea
							name={field.id}
							maxlength={field.maxLength}
							placeholder={field.placeholder}
							required={field.required}
						></textarea>
					</If>
					<If cond={field.isSelect}>
						<select name={field.id} required={field.required}>
							<option value="">{"Choose " + field.label}</option>
							<Each of={field.options} as="option">
								<option value={option.value}>{option.label}</option>
							</Each>
						</select>
					</If>
					<If cond={field.isInput}>
						<input
							type={field.inputType}
							name={field.id}
							maxlength={field.maxLength}
							placeholder={field.placeholder}
							required={field.required}
						></input>
					</If>
					<If cond={field.help != ""}>
						<small>{field.help}</small>
					</If>
				</label>
			</Each>
		</fieldset>
		<footer>
			<p>
				This is a client-only walkthrough. Try the fields and actions; no request, email, or workspace record is created.
			</p>
			<button class="button" type="button" onClick={saveDraft}>Save draft</button>
			<button class="button button-primary" type="submit">
				Submit proposal
				<span aria-hidden="true">→</span>
			</button>
		</footer>
	</form>
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
				<If cond={data.form.confirmation}>
					<span>
						<i aria-hidden="true">✓</i>
						Confirmation message
					</span>
				</If>
				<If cond={data.form.redirect}>
					<span>
						<i aria-hidden="true">✓</i>
						Secure speaker portal
					</span>
				</If>
				<span>
					<i aria-hidden="true">✓</i>
					{data.form.reviewProcess}
				</span>
			</div>
		</aside>
		<section class="submission-form-panel">
			<header>
				<p class="panel-kicker">
					{data.event.year}
					call for speakers
				</p>
				<h2>Tell us what you have learned.</h2>
				<p>{data.form.body}</p>
			</header>
			<If cond={data.readOnlyPreview}>
				<div class="closed-notice" role="status">
					<strong>Submission journey preview.</strong>
					<p>
						Try the live call’s questions and conditional structure below. Save draft and Submit proposal are simulated in your browser only; nothing is sent or saved.
					</p>
				</div>
			</If>
			<If cond={!data.form.open}>
				<div class="closed-notice">
					<strong>This call is closed.</strong>
					<p>
						Contact the program team if you believe this is an error.
					</p>
				</div>
			</If>
			<If cond={data.form.open && !data.readOnlyPreview}>
				<ActionForm class="public-form" actionName="submitProposal" aria-describedby="submission-form-status">
					<input type="hidden" name="csrf_token" value={csrf.token}></input>
					<input type="hidden" name="form_id" value={data.form.id}></input>
					<input type="hidden" name="draft_id" value={data.draft.id}></input>
					<input type="hidden" name="draft_key" value={data.draft.key}></input>
					<p class="form-status" id="submission-form-status" role="alert" tabindex="-1">{action.message}</p>
					<If cond={data.draft.active}>
						<p class="form-status" role="status">
							You are editing a saved draft. Submitting it will send it into review.
						</p>
					</If>
					<fieldset>
						<legend>
							<span>01</span>
							Proposal
						</legend>
						<Each of={data.proposalFields} as="field">
							<If cond={field.isConditionalSource}>
								<ConditionalFormatFields
									source={field}
									sourceValue={actions.submitProposal.name != "" ? "" + actions.submitProposal.values[field.id] : field.value}
									sourceError={"" + actions.submitProposal.fieldErrors[field.id]}
								></ConditionalFormatFields>
							</If>
							<If cond={!field.isConditionalTarget && !field.isConditionalSource}>
								<FormFieldRow
									field={field}
									value={actions.submitProposal.name != "" ? actions.submitProposal.values[field.id] : field.value}
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
							<If cond={field.isConditionalSource}>
								<ConditionalFormatFields
									source={field}
									sourceValue={actions.submitProposal.name != "" ? "" + actions.submitProposal.values[field.id] : field.value}
									sourceError={"" + actions.submitProposal.fieldErrors[field.id]}
								></ConditionalFormatFields>
							</If>
							<If cond={!field.isConditionalTarget && !field.isConditionalSource}>
								<FormFieldRow
									field={field}
									value={actions.submitProposal.name != "" ? actions.submitProposal.values[field.id] : field.value}
									error={actions.submitProposal.fieldErrors[field.id]}
								></FormFieldRow>
							</If>
						</Each>
					</fieldset>
					<footer>
						<p>
							By submitting, you agree to the event code of conduct. Payments are not collected.
						</p>
						<button class="button" type="submit" formaction={actionPath("saveDraft")}>Save draft</button>
						<button class="button button-primary" type="submit">
							Submit proposal
							<span aria-hidden="true">→</span>
						</button>
					</footer>
				</ActionForm>
			</If>
			<If cond={data.form.open && data.readOnlyPreview}>
				<PreviewSubmissionForm proposalFields={data.proposalFields} participantFields={data.participantFields}></PreviewSubmissionForm>
			</If>
		</section>
	</main>
}
