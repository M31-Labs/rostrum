package submit

func FieldError(props any) Node {
	return <p class="form-error" id={props.id + "-error"} aria-live="polite">{props.message}</p>
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
						<label>
							<span>
								Session title
								<b>Required</b>
							</span>
							<input
								name="title"
								value={actions.submitProposal.values.title}
								maxlength="120"
								placeholder="A concrete title that tells us what changes"
								aria-describedby="title-error"
								required
							></input>
							<FieldError id="title" message={actions.submitProposal.fieldErrors.title}></FieldError>
						</label>
						<label>
							<span>
								Abstract
								<b>Required</b>
							</span>
							<textarea
								name="abstract"
								maxlength="1600"
								placeholder="The problem, your approach, and the attendee outcome"
								aria-describedby="abstract-error"
								required
							>
								{actions.submitProposal.values.abstract}
							</textarea>
							<FieldError id="abstract" message={actions.submitProposal.fieldErrors.abstract}></FieldError>
						</label>
						<div class="form-grid-three">
							<ConditionalFormatFields
								formats={data.formats}
								formatValue={actions.submitProposal.values.format}
								formatError={actions.submitProposal.fieldErrors.format}
								workshopNeeds={actions.submitProposal.values.workshop_needs}
								workshopError={actions.submitProposal.fieldErrors.workshop_needs}
								conditionalWhy={data.form.conditionalWhy}
							></ConditionalFormatFields>
							<label>
								<span>
									Category
									<b>Required</b>
								</span>
								<select name="category" aria-describedby="category-error" required>
									<option value="">Choose category</option>
									<Each of={data.categories} as="category">
										<option value={category.id} selected={category.id == actions.submitProposal.values.category}>{category.name}</option>
									</Each>
								</select>
								<FieldError id="category" message={actions.submitProposal.fieldErrors.category}></FieldError>
							</label>
							<label>
								<span>
									Audience level
									<b>Required</b>
								</span>
								<select name="level" aria-describedby="level-error" required>
									<option value="">Choose level</option>
									<Each of={data.levels} as="option">
										<option value={option.value} selected={option.value == actions.submitProposal.values.level}>{option.label}</option>
									</Each>
								</select>
								<FieldError id="level" message={actions.submitProposal.fieldErrors.level}></FieldError>
							</label>
						</div>
						<label>
							<span>
								Three takeaways
								<b>Required</b>
							</span>
							<textarea
								name="topics"
								maxlength="500"
								placeholder="One concrete takeaway per line"
								aria-describedby="topics-error"
								required
							>{actions.submitProposal.values.topics}</textarea>
							<FieldError id="topics" message={actions.submitProposal.fieldErrors.topics}></FieldError>
						</label>
					</fieldset>
					<fieldset>
						<legend>
							<span>02</span>
							Participant
						</legend>
						<div class="form-grid-two">
							<label>
								<span>
									First name
									<b>Required</b>
								</span>
								<input
									name="first_name"
									value={actions.submitProposal.values.first_name}
									autocomplete="given-name"
									aria-describedby="first_name-error"
									required
								></input>
								<FieldError id="first_name" message={actions.submitProposal.fieldErrors.first_name}></FieldError>
							</label>
							<label>
								<span>
									Last name
									<b>Required</b>
								</span>
								<input
									name="last_name"
									value={actions.submitProposal.values.last_name}
									autocomplete="family-name"
									aria-describedby="last_name-error"
									required
								></input>
								<FieldError id="last_name" message={actions.submitProposal.fieldErrors.last_name}></FieldError>
							</label>
						</div>
						<label>
							<span>
								Email
								<b>Required</b>
							</span>
							<input
								type="email"
								name="email"
								value={actions.submitProposal.values.email}
								autocomplete="email"
								placeholder="you@example.com"
								aria-describedby="email-error"
								required
							></input>
							<FieldError id="email" message={actions.submitProposal.fieldErrors.email}></FieldError>
						</label>
						<div class="form-grid-two">
							<label>
								<span>Role</span>
								<input name="role" value={actions.submitProposal.values.role} autocomplete="organization-title"></input>
							</label>
							<label>
								<span>Company or project</span>
								<input name="company" value={actions.submitProposal.values.company} autocomplete="organization"></input>
							</label>
						</div>
						<label>
							<span>Short biography</span>
							<textarea
								name="biography"
								maxlength="800"
								placeholder="You can finish this in the portal after submitting"
							>
								{actions.submitProposal.values.biography}
							</textarea>
						</label>
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
