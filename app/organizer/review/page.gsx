package review

import "github.com/m31-labs/rostrum/app/organizer/embeds"

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
		<section id="candidates" class="panel table-panel">
			<header class="panel-header">
				<div>
					<p class="panel-kicker">
						Active ·
						{data.activePlan.name}
					</p>
					<h2>Candidate scorecard</h2>
				</div>
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
		<section class="panel reviewer-panel">
			<header class="panel-header">
				<div>
					<p class="panel-kicker">Assignment pool</p>
					<h2>Reviewers</h2>
				</div>
				<span>
					{data.reviewerCount}
					total
				</span>
			</header>
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
								<embeds.EmbedClipboard code={reviewer.link}></embeds.EmbedClipboard>
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
