package review

import "m31labs.dev/gosx/browser"

// ReviewLinkClipboard renders one reviewer's signed /review/{token} URL as
// visible, selectable text plus a "Copy" button bound by the framework
// clipboard runtime. It is defined here, inside the review package, rather
// than imported from app/organizer/embeds: an island reference across a
// file-route package boundary does not resolve at render time (the file
// router discovers islands only from the single .gsx program that declares
// them), so a cross-package <embeds.EmbedClipboard> tag rendered as an
// unresolved data-gosx-component placeholder instead of the reviewer link.
// This component copies EmbedClipboard's structure (app/organizer/embeds)
// so the two stay visually identical.
//gosx:island
func ReviewLinkClipboard(props any) Node {
	label := signal.New("Copy link")
	status := signal.New("")
	copy := func() {
		label.Set(browser.ClipboardWrite(code) ? "Copied" : "Copy failed")
		status.Set(label.Get() == "Copied" ? "Review link copied to the clipboard." : "Could not copy the review link. Select the link and copy it manually.")
	}
	return <div class="embed-code-block">
		<pre tabindex="0">
			<code>{code}</code>
		</pre>
		<button class="button button-compact" type="button" onClick={copy}>{label.Get()}</button>
		<span class="sr-only" role="status" aria-live="polite">{status.Get()}</span>
	</div>
}

func ReviewPlanCard(props any) Node {
	return <article class="review-plan-card">
		<div class="summary-top">
			<span class={"status-pill status-" + props.tone}>{props.status}</span>
			<span class="mono">
				Round
				{props.round}
			</span>
		</div>
		<h2>{props.name}</h2>
		<p>{props.instructions}</p>
		<div class="review-progress">
			<div>
				<span>Coverage</span>
				<strong>
					{props.coverage}
					%
				</strong>
			</div>
			<div class="progress-track">
				<span class="progress-fill fill-positive" style={"--progress: " + props.coverageStyle}></span>
			</div>
			<small>
				{props.completed}
				of
				{props.expected}
				evaluations · due
				{props.due}
			</small>
		</div>
		<div class="plan-flags">
			<span>
				{props.reviewers}
				reviewers
			</span>
			<span>
				{props.proposals}
				proposals
			</span>
			<If cond={props.anonymous}>
				<span>Anonymous</span>
			</If>
		</div>
		<details class="rubric-details">
			<summary>Weighted rubric</summary>
			<div>
				<Each of={props.criteria} as="criterion">
					<article>
						<strong>{criterion.name}</strong>
						<span>{criterion.weight}</span>
						<p>{criterion.description}</p>
					</article>
				</Each>
			</div>
		</details>
	</article>
}

func ReviewMethodDialog() Node {
	return <div class="review-method-control">
		<button
			class="button"
			type="button"
			data-gosx-disclosure-target="#review-method"
			aria-haspopup="dialog"
			aria-controls="review-method"
			aria-expanded="false"
		>Review method</button>
		<div class="modal-dialog-backdrop" data-gosx-disclosure-backdrop="#review-method" hidden></div>
		<section
			id="review-method"
			class="modal-dialog"
			data-gosx-disclosure
			role="dialog"
			aria-modal="true"
			aria-labelledby="review-method-title"
			hidden
		>
			<button
				id="review-method-close"
				class="dialog-close"
				type="button"
				data-gosx-disclosure-close="#review-method"
				data-gosx-disclosure-initial-focus
				aria-label="Close"
			>×</button>
			<p class="eyebrow">Review governance</p>
			<h2 id="review-method-title">Assistance, not autopilot.</h2>
			<ol>
				<li>
					Program staff define the weighted rubric.
				</li>
				<li>
					Every assigned reviewer scores the same criteria.
				</li>
				<li>
					Every recorded score keeps its reviewer, timestamp, and comments.
				</li>
				<li>
					Only program staff change acceptance status.
				</li>
			</ol>
		</section>
	</div>
}

func Page() Node {
	return <main id="main-content" class="workspace-page" data-section={data.section}>
		<header class="workspace-header">
			<div>
				<p class="eyebrow">Defensible decisions</p>
				<h1>Multi-round review</h1>
				<p>
					Human judgment stays primary. Every score follows the same weighted rubric.
				</p>
			</div>
			<div class="workspace-header-actions">
				<ReviewMethodDialog></ReviewMethodDialog>
				<a class="button button-primary" href="#candidates">Open active round</a>
			</div>
		</header>
		<p class="flash-message">{flash.notice}</p>
		<section class="review-plan-grid">
			<Each of={data.plans} as="plan">
				<ReviewPlanCard {...plan}></ReviewPlanCard>
			</Each>
		</section>
		<section id="plan-manager" class="panel review-plan-manager">
			<header class="panel-header">
				<div>
					<p class="panel-kicker">Round operations</p>
					<h2>
						Plans, rubric, roster, and assignment impact
					</h2>
				</div>
				<span>Scores stay attributable</span>
			</header>
			<p>
				Open one round at a time. Editing a roster never deletes a review; changes to a rubric with recorded scores require a new round.
			</p>
			<ActionForm class="review-plan-create-form" actionName="createReviewPlan">
				<input type="hidden" name="csrf_token" value={csrf.token}></input>
				<p class="form-status" role="status" aria-live="polite">{actions.createReviewPlan.message}</p>
				<div class="form-grid-two">
					<label>
						<span>Plan name</span>
						<input name="name" maxlength="160" required placeholder="e.g. Round 3 — programming committee"></input>
						<p class="form-error" data-gosx-field-error="name" aria-live="polite"></p>
					</label>
					<label>
						<span>Round</span>
						<input type="number" name="round" min="1" max="99" value="1" required></input>
						<p class="form-error" data-gosx-field-error="round" aria-live="polite"></p>
					</label>
				</div>
				<div class="form-grid-two">
					<label>
						<span>Initial state</span>
						<select name="status">
							<option value="draft">Draft</option>
							<option value="open">Open now</option>
						</select>
						<p class="form-error" data-gosx-field-error="status" aria-live="polite"></p>
					</label>
					<label>
						<span>Due date</span>
						<input type="datetime-local" name="due_at" required></input>
						<p class="form-error" data-gosx-field-error="due_at" aria-live="polite"></p>
					</label>
				</div>
				<label>
					<span>Instructions</span>
					<textarea name="instructions" maxlength="4000" placeholder="How should reviewers apply this rubric?"></textarea>
					<p class="form-error" data-gosx-field-error="instructions" aria-live="polite"></p>
				</label>
				<label>
					<span>
						Rubric — one line per criterion: id|name|description|weight|max
					</span>
					<textarea name="rubric" required>{data.defaultRubric}</textarea>
					<p class="form-error" data-gosx-field-error="rubric" aria-live="polite"></p>
				</label>
				<div class="form-grid-two">
					<label>
						<span>Human evaluations per proposal</span>
						<input type="number" name="evaluations_per_item" min="1" max="20" value="2" required></input>
						<p class="form-error" data-gosx-field-error="evaluations_per_item" aria-live="polite"></p>
					</label>
					<div class="checkbox-stack">
						<label class="checkbox-control">
							<input type="checkbox" name="anonymous"></input>
							<span>Anonymous reviewer view</span>
						</label>
						<label class="checkbox-control">
							<input type="checkbox" name="weekly_reminders"></input>
							<span>Weekly reminders</span>
						</label>
						<label class="checkbox-control">
							<input type="checkbox" name="include_files"></input>
							<span>Include uploaded files</span>
						</label>
					</div>
				</div>
				<button class="button button-primary" type="submit">Create review plan</button>
			</ActionForm>
			<div class="review-plan-manager-list">
				<Each of={data.plans} as="plan">
					<article class="review-plan-manager-card">
						<header>
							<div>
								<strong>{plan.name}</strong>
								<small>
									Round
									{plan.round}
									·
									{plan.status}
								</small>
							</div>
							<span class="status-pill status-neutral">
								{plan.completed}
								recorded score(s)
							</span>
						</header>
						<div class="review-impact-preview" role="status">
							<strong>Impact preview</strong>
							<span>
								{plan.activeAssignments}
								active assignment(s)
							</span>
							<span>
								{plan.unfilledAssignments}
								unfilled target slot(s)
							</span>
							<span>
								{plan.recusalConflicts}
								recusal pairing(s) excluded
							</span>
							<span>
								Removing a reviewer preserves
								{plan.completed}
								recorded score(s).
							</span>
						</div>
						<If cond={plan.editable}>
							<ActionForm class="review-plan-update-form" actionName="updateReviewPlan">
								<input type="hidden" name="csrf_token" value={csrf.token}></input>
								<input type="hidden" name="plan_id" value={plan.id}></input>
								<p class="form-status" role="status" aria-live="polite">{actions.updateReviewPlan.message}</p>
								<div class="form-grid-two">
									<label>
										<span>Plan name</span>
										<input name="name" maxlength="160" value={plan.name} required></input>
										<p class="form-error" data-gosx-field-error="name" aria-live="polite"></p>
									</label>
									<label>
										<span>Round</span>
										<input type="number" name="round" min="1" max="99" value={plan.round} required></input>
										<p class="form-error" data-gosx-field-error="round" aria-live="polite"></p>
									</label>
								</div>
								<div class="form-grid-two">
									<label>
										<span>State</span>
										<select name="status">
											<option value="draft" selected={plan.statusValue == "draft"}>Draft</option>
											<option value="open" selected={plan.statusValue == "open" || plan.statusValue == "active"}>Open</option>
											<option value="closed" selected={plan.statusValue == "closed"}>Closed</option>
										</select>
										<p class="form-error" data-gosx-field-error="status" aria-live="polite"></p>
									</label>
									<label>
										<span>Due date</span>
										<input type="datetime-local" name="due_at" value={plan.dueInput} required></input>
										<p class="form-error" data-gosx-field-error="due_at" aria-live="polite"></p>
									</label>
								</div>
								<label>
									<span>Instructions</span>
									<textarea name="instructions" maxlength="4000">{plan.instructions}</textarea>
									<p class="form-error" data-gosx-field-error="instructions" aria-live="polite"></p>
								</label>
								<label>
									<span>Rubric</span>
									<textarea name="rubric" required>{plan.rubric}</textarea>
									<p class="form-error" data-gosx-field-error="rubric" aria-live="polite"></p>
								</label>
								<div class="form-grid-two">
									<label>
										<span>Human evaluations per proposal</span>
										<input
											type="number"
											name="evaluations_per_item"
											min="1"
											max="20"
											value={plan.evaluationsPerItem}
											required
										></input>
										<p class="form-error" data-gosx-field-error="evaluations_per_item" aria-live="polite"></p>
									</label>
									<div class="checkbox-stack">
										<label class="checkbox-control">
											<input type="checkbox" name="anonymous" checked={plan.anonymous}></input>
											<span>Anonymous reviewer view</span>
										</label>
										<label class="checkbox-control">
											<input type="checkbox" name="weekly_reminders" checked={plan.weeklyReminders}></input>
											<span>Weekly reminders</span>
										</label>
										<label class="checkbox-control">
											<input type="checkbox" name="include_files" checked={plan.includeFiles}></input>
											<span>Include uploaded files</span>
										</label>
									</div>
								</div>
								<button class="button button-compact" type="submit">Save review plan</button>
							</ActionForm>
							<div class="review-roster-editor">
								<h3>Roster</h3>
								<div class="review-roster-list">
									<Each of={plan.roster} as="reviewer">
										<article>
											<div>
												<strong>{reviewer.name}</strong>
												<small>
													{reviewer.kind}
													·
													{reviewer.scoreCount}
													score(s) ·
													{reviewer.assignments}
													active assignment(s)
												</small>
											</div>
											<ActionForm actionName="removePlanReviewer">
												<input type="hidden" name="csrf_token" value={csrf.token}></input>
												<input type="hidden" name="plan_id" value={plan.id}></input>
												<input type="hidden" name="reviewer_id" value={reviewer.id}></input>
												<p class="form-status" role="status" aria-live="polite">{actions.removePlanReviewer.message}</p>
												<button class="button button-compact" type="submit">Remove from roster</button>
											</ActionForm>
										</article>
									</Each>
								</div>
								<ActionForm class="review-roster-add" actionName="addPlanReviewer">
									<input type="hidden" name="csrf_token" value={csrf.token}></input>
									<input type="hidden" name="plan_id" value={plan.id}></input>
									<label>
										<span>Add active reviewer</span>
										<select name="reviewer_id" required>
											<Each of={data.activeReviewers} as="reviewer">
												<option value={reviewer.id}>
													{reviewer.name}
													·
													{reviewer.kind}
												</option>
											</Each>
										</select>
										<p class="form-error" data-gosx-field-error="reviewer_id" aria-live="polite"></p>
									</label>
									<p class="form-status" role="status" aria-live="polite">{actions.addPlanReviewer.message}</p>
									<button class="button button-compact" type="submit">Add reviewer</button>
								</ActionForm>
								<ActionForm class="review-auto-assign" actionName="autoAssignReviewers">
									<input type="hidden" name="csrf_token" value={csrf.token}></input>
									<input type="hidden" name="plan_id" value={plan.id}></input>
									<p class="form-status" role="status" aria-live="polite">{actions.autoAssignReviewers.message}</p>
									<button class="button button-compact" type="submit">Create balanced assignments</button>
								</ActionForm>
							</div>
						</If>
						<If cond={!plan.editable}>
							<p class="closed-notice">
								This plan is closed. Its roster, rubric, assignments, and scores remain as historical evidence.
							</p>
						</If>
						<If cond={plan.assignmentsManaged}>
							<details class="assignment-provenance-list">
								<summary>
									{plan.activeAssignments}
									active assignment(s) with provenance
								</summary>
								<Each of={plan.assignments} as="assignment">
									<article>
										<span>
											<strong>{assignment.proposal}</strong>
											→
											{assignment.reviewer}
											<small>
												{assignment.source}
												·
												{assignment.assignedAt}
											</small>
										</span>
										<If cond={plan.editable}>
											<ActionForm actionName="unassignReview">
												<input type="hidden" name="csrf_token" value={csrf.token}></input>
												<input type="hidden" name="assignment_id" value={assignment.id}></input>
												<p class="form-status" role="status" aria-live="polite">{actions.unassignReview.message}</p>
												<button class="button button-compact" type="submit">Reassign</button>
											</ActionForm>
										</If>
									</article>
								</Each>
							</details>
						</If>
					</article>
				</Each>
			</div>
		</section>
		<section id="candidates" class="panel table-panel">
			<header class="panel-header">
				<div>
					<p class="panel-kicker">
						Active ·
						{data.activePlan.name}
					</p>
					<h2>Candidate scorecard</h2>
				</div>
				<ActionForm actionName="assignPendingToActivePlan">
					<input type="hidden" name="csrf_token" value={csrf.token}></input>
					<button class="button button-compact" type="submit">Assign pending proposals</button>
					<p class="form-status" role="status" aria-live="polite">
						{actions.assignPendingToActivePlan.message}
					</p>
				</ActionForm>
			</header>
			<div class="review-table" role="table" aria-label="Round two candidate scores">
				<div class="review-head" role="row">
					<span>Proposal</span>
					<span>Category</span>
					<span>Reviews</span>
					<span>Weighted score</span>
					<span>Recommendation</span>
				</div>
				<Each of={data.candidates} as="candidate">
					<article class="review-row" role="row">
						<div role="cell">
							<a href={"/organizer/submissions/" + candidate.id} data-gosx-link>
								<strong>{candidate.title}</strong>
							</a>
							<small>{candidate.speaker}</small>
						</div>
						<span role="cell">{candidate.category}</span>
						<span class="mono" role="cell">
							{candidate.evaluationCount}
							/
							{candidate.targetCount}
						</span>
						<strong class="score-value" role="cell">
							{candidate.score}
							<small>/ 5</small>
						</strong>
						<div role="cell">
							<span class={"status-pill status-" + candidate.tone}>{candidate.status}</span>
							<small>{candidate.recommendations}</small>
						</div>
					</article>
				</Each>
			</div>
			<If cond={data.hasActivePlan}>
				<ActionForm class="review-manual-assignment" actionName="assignReview">
					<input type="hidden" name="csrf_token" value={csrf.token}></input>
					<input type="hidden" name="plan_id" value={data.activePlan.id}></input>
					<div class="form-grid-two">
						<label>
							<span>Assign proposal</span>
							<select name="submission_id" required>
								<Each of={data.candidates} as="candidate">
									<option value={candidate.id}>{candidate.title}</option>
								</Each>
							</select>
							<p class="form-error" data-gosx-field-error="submission_id" aria-live="polite"></p>
						</label>
						<label>
							<span>To human reviewer</span>
							<select name="reviewer_id" required>
								<Each of={data.humanReviewers} as="reviewer">
									<option value={reviewer.id}>{reviewer.name}</option>
								</Each>
							</select>
							<p class="form-error" data-gosx-field-error="reviewer_id" aria-live="polite"></p>
						</label>
					</div>
					<p class="form-status" role="status" aria-live="polite">{actions.assignReview.message}</p>
					<button class="button button-compact" type="submit">Add manual assignment</button>
				</ActionForm>
			</If>
		</section>
		<section class="panel review-entry-panel">
			<header class="panel-header">
				<div>
					<p class="panel-kicker">Human judgment</p>
					<h2>Record a rubric review</h2>
				</div>
			</header>
			<p>
				Scored here on behalf of the reviewer by program staff. A reviewer with a review link (below) can score their own proposals directly instead.
			</p>
			<ActionForm class="review-entry-form" actionName="saveReview">
				<input type="hidden" name="csrf_token" value={csrf.token}></input>
				<input type="hidden" name="plan_id" value={data.activePlan.id}></input>
				<p class="form-error" data-gosx-field-error="plan_id" aria-live="polite"></p>
				<p class="form-status" role="alert" aria-live="assertive">{actions.saveReview.message}</p>
				<div class="form-grid-two">
					<label>
						<span>Proposal</span>
						<select name="submission_id" required>
							<Each of={data.candidates} as="candidate">
								<option value={candidate.id}>{candidate.title}</option>
							</Each>
						</select>
						<p class="form-error" data-gosx-field-error="submission_id" aria-live="polite"></p>
					</label>
					<label>
						<span>Reviewer</span>
						<select name="reviewer_id" required>
							<Each of={data.humanReviewers} as="reviewer">
								<option value={reviewer.id}>{reviewer.name}</option>
							</Each>
						</select>
						<p class="form-error" data-gosx-field-error="reviewer_id" aria-live="polite"></p>
					</label>
				</div>
				<fieldset>
					<legend>Weighted criteria</legend>
					<div class="rubric-score-grid">
						<Each of={data.activePlan.criteria} as="criterion">
							<label>
								<span>
									{criterion.name}
									<small>{criterion.weight}</small>
								</span>
								<input
									type="number"
									name={"score_" + criterion.id}
									min="0"
									max={criterion.max}
									step="0.1"
									required
								></input>
								<p class="form-error" data-gosx-field-error={"score_" + criterion.id} aria-live="polite"></p>
								<small>{criterion.description}</small>
							</label>
						</Each>
					</div>
				</fieldset>
				<div class="form-grid-two">
					<label>
						<span>Recommendation</span>
						<select name="recommendation" required>
							<option value="strong_yes">Strong yes</option>
							<option value="yes">Yes</option>
							<option value="maybe">Maybe</option>
							<option value="no">No</option>
							<option value="strong_no">Strong no</option>
						</select>
						<p class="form-error" data-gosx-field-error="recommendation" aria-live="polite"></p>
					</label>
					<label>
						<span>Decision context</span>
						<textarea
							name="comments"
							minlength="20"
							required
							placeholder="Most important strength, risk, and follow-up question"
						></textarea>
						<p class="form-error" data-gosx-field-error="comments" aria-live="polite"></p>
					</label>
				</div>
				<button class="button button-primary" type="submit">Save human review</button>
			</ActionForm>
		</section>
		<section id="reviewer-manager" class="panel reviewer-panel">
			<header class="panel-header">
				<div>
					<p class="panel-kicker">Assignment pool</p>
					<h2>Reviewers</h2>
				</div>
				<span>
					{data.reviewerCount}
					active ·
					{data.reviewerTotal}
					total
				</span>
			</header>
			<ActionForm class="reviewer-create-form" actionName="createReviewer">
				<input type="hidden" name="csrf_token" value={csrf.token}></input>
				<p class="form-status" role="status" aria-live="polite">{actions.createReviewer.message}</p>
				<div class="form-grid-two">
					<label>
						<span>Name</span>
						<input name="name" maxlength="160" required></input>
						<p class="form-error" data-gosx-field-error="name" aria-live="polite"></p>
					</label>
					<label>
						<span>Kind</span>
						<select name="kind">
							<option value="human">Human reviewer</option>
							<option value="virtual">Virtual lens</option>
						</select>
						<p class="form-error" data-gosx-field-error="kind" aria-live="polite"></p>
					</label>
				</div>
				<div class="form-grid-two">
					<label>
						<span>Email</span>
						<input type="email" name="email"></input>
						<p class="form-error" data-gosx-field-error="email" aria-live="polite"></p>
					</label>
					<label>
						<span>Company / organization</span>
						<input name="company" maxlength="160"></input>
					</label>
				</div>
				<label>
					<span>Expertise tags (comma-separated)</span>
					<input name="expertise" maxlength="500" placeholder="agents, governance, accessibility"></input>
					<p class="form-error" data-gosx-field-error="expertise" aria-live="polite"></p>
				</label>
				<button class="button button-compact" type="submit">Add reviewer</button>
			</ActionForm>
			<div class="reviewer-grid">
				<Each of={data.reviewers} as="reviewer">
					<article class="reviewer-card">
						<span class="avatar">{reviewer.initials}</span>
						<div>
							<strong>{reviewer.name}</strong>
							<small>
								{reviewer.kind}
								·
								{reviewer.expertise}
							</small>
						</div>
						<span class="mono">
							{reviewer.completed}
							done
						</span>
						<If cond={reviewer.active}>
							<details class="reviewer-editor">
								<summary>Edit reviewer</summary>
								<ActionForm actionName="updateReviewer">
									<input type="hidden" name="csrf_token" value={csrf.token}></input>
									<input type="hidden" name="reviewer_id" value={reviewer.id}></input>
									<p class="form-status" role="status" aria-live="polite">{actions.updateReviewer.message}</p>
									<label>
										<span>Name</span>
										<input name="name" value={reviewer.name} required></input>
										<p class="form-error" data-gosx-field-error="name" aria-live="polite"></p>
									</label>
									<label>
										<span>Kind</span>
										<select name="kind">
											<option value="human" selected={reviewer.kindValue == "human"}>Human reviewer</option>
											<option value="virtual" selected={reviewer.kindValue == "virtual"}>Virtual lens</option>
										</select>
										<p class="form-error" data-gosx-field-error="kind" aria-live="polite"></p>
									</label>
									<label>
										<span>Email</span>
										<input type="email" name="email" value={reviewer.email}></input>
										<p class="form-error" data-gosx-field-error="email" aria-live="polite"></p>
									</label>
									<label>
										<span>Company</span>
										<input name="company" value={reviewer.company}></input>
									</label>
									<label>
										<span>Expertise</span>
										<input name="expertise" value={reviewer.expertiseInput}></input>
										<p class="form-error" data-gosx-field-error="expertise" aria-live="polite"></p>
									</label>
									<button class="button button-compact" type="submit">Save reviewer</button>
								</ActionForm>
								<ActionForm actionName="retireReviewer">
									<input type="hidden" name="csrf_token" value={csrf.token}></input>
									<input type="hidden" name="reviewer_id" value={reviewer.id}></input>
									<p class="form-status" role="status" aria-live="polite">{actions.retireReviewer.message}</p>
									<button class="button button-compact" type="submit">Retire reviewer</button>
								</ActionForm>
							</details>
						</If>
						<If cond={reviewer.retired}>
							<small>
								Retired
								{reviewer.retiredAt}
								; historical scores remain attributable.
							</small>
						</If>
					</article>
				</Each>
			</div>
		</section>
		<section class="panel reviewer-panel">
			<header class="panel-header">
				<div>
					<p class="panel-kicker">Reviewer sign-in</p>
					<h2>Reviewer links</h2>
				</div>
				<span>No reviewer accounts needed</span>
			</header>
			<div class="reviewer-grid">
				<Each of={data.reviewerLinks} as="reviewer">
					<article class="reviewer-card">
						<span class="avatar">{reviewer.initials}</span>
						<div>
							<strong>{reviewer.name}</strong>
							<small>{reviewer.kind}</small>
						</div>
						<If cond={reviewer.canReview}>
							<details class="embed-code-disclosure">
								<summary>Copy review link</summary>
								<ReviewLinkClipboard code={reviewer.link}></ReviewLinkClipboard>
							</details>
						</If>
						<If cond={!reviewer.canReview}>
							<span class="mono">No sign-in needed</span>
						</If>
					</article>
				</Each>
			</div>
		</section>
	</main>
}
