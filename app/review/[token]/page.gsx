package review

func ReviewUnavailable() Node {
	return <section class="portal-section portal-unavailable">
		<p class="panel-kicker">Reviewer link</p>
		<h1>Check your email for your review link</h1>
		<p>
			This link is missing or no longer active. Open the review link from your invitation email to continue, or contact the program team for a new one.
		</p>
	</section>
}

func ScoreForm(props any) Node {
	return <ActionForm class="review-entry-form score-form" actionName="scoreSubmission">
		<input type="hidden" name="csrf_token" value={csrf.token}></input>
		<input type="hidden" name="plan_id" value={data.plan.id}></input>
		<input type="hidden" name="submission_id" value={props.id}></input>
		<p class="form-error" data-gosx-field-error="submission_id" aria-live="polite"></p>
		<p class="form-status" role="status" aria-live="polite">{action.message}</p>
		<fieldset>
			<legend>Weighted criteria</legend>
			<div class="rubric-score-grid">
				<Each of={props.scores} as="score">
					<label>
						<span>
							{score.name}
							<small>{score.weight}</small>
						</span>
						<input
							type="number"
							name={"score_" + score.id}
							min="0"
							max={score.max}
							step="0.1"
							value={score.value}
							required
						></input>
						<p class="form-error" data-gosx-field-error={"score_" + score.id} aria-live="polite"></p>
						<small>{score.description}</small>
					</label>
				</Each>
			</div>
		</fieldset>
		<div class="form-grid-two">
			<label>
				<span>Recommendation</span>
				<select name="recommendation" required>
					<option value="strong_yes" selected={props.recommendation == "strong_yes"}>Strong yes</option>
					<option value="yes" selected={props.recommendation == "yes"}>Yes</option>
					<option value="maybe" selected={props.recommendation == "maybe"}>Maybe</option>
					<option value="no" selected={props.recommendation == "no"}>No</option>
					<option value="strong_no" selected={props.recommendation == "strong_no"}>Strong no</option>
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
				>{props.comments}</textarea>
				<p class="form-error" data-gosx-field-error="comments" aria-live="polite"></p>
			</label>
		</div>
		<button class="button button-primary" type="submit">
			<If cond={props.reviewed}>Update your review</If>
			<If cond={!props.reviewed}>Save your review</If>
		</button>
	</ActionForm>
}

func ScoreSnapshot(props any) Node {
	return <div class="review-entry-form score-form">
		<fieldset>
			<legend>Weighted criteria</legend>
			<dl class="detail-answers">
				<Each of={props.scores} as="score">
					<div>
						<dt>{score.name + " · " + score.weight}</dt>
						<dd>
							<If cond={score.value != ""}>{score.value + " / " + score.max}</If>
							<If cond={score.value == ""}>Not scored</If>
						</dd>
					</div>
				</Each>
			</dl>
		</fieldset>
		<If cond={props.reviewed}>
			<dl class="detail-answers">
				<div>
					<dt>Recommendation</dt>
					<dd>{props.recommendation}</dd>
				</div>
				<div>
					<dt>Decision context</dt>
					<dd>{props.comments}</dd>
				</div>
			</dl>
		</If>
		<If cond={!props.reviewed}>
			<p class="form-note">
				No review has been saved for this assignment.
			</p>
		</If>
	</div>
}

func SubmissionCard(props any) Node {
	return <article class="panel score-card">
		<header class="panel-header">
			<div>
				<p class="panel-kicker">
					{props.category}
					·
					{props.format}
					·
					{props.level}
				</p>
				<h2>{props.title}</h2>
				<p>{props.speaker}</p>
			</div>
			<If cond={props.reviewed}>
				<span class="status-pill status-positive">
					Reviewed ·
					{props.updated}
				</span>
			</If>
		</header>
		<p class="detail-abstract">{props.abstract}</p>
		<If cond={!data.readOnlyPreview}>
			<ScoreForm {...props}></ScoreForm>
		</If>
		<If cond={data.readOnlyPreview}>
			<ScoreSnapshot {...props}></ScoreSnapshot>
		</If>
	</article>
}

func Page() Node {
	return <main id="main-content" class="portal-main">
		<If cond={!data.available}>
			<ReviewUnavailable></ReviewUnavailable>
		</If>
		<If cond={data.available}>
			<If cond={data.readOnlyPreview}>
				<div class="closed-notice" role="status">
					<strong>Reviewer journey preview.</strong>
					<p>
						Assignments, rubric, and saved evidence are visible; scoring controls are not shown.
					</p>
				</div>
			</If>
			<p class="portal-flash">{flash.notice}</p>
			<section class="portal-hero">
				<div class="portal-profile-summary">
					<span class="avatar portal-avatar">{data.reviewer.initials}</span>
					<div>
						<p class="eyebrow">Reviewer link</p>
						<h1>
							{"Welcome, " + data.reviewer.name + "."}
						</h1>
						<If cond={data.hasActiveRound}>
							<p>
								{data.plan.name}
								· due
								{data.plan.due}
							</p>
						</If>
					</div>
				</div>
			</section>
			<If cond={!data.hasActiveRound}>
				<section class="panel empty-state">
					<strong>No review round is open right now.</strong>
					<p>
						Check back once the program team opens the next round.
					</p>
				</section>
			</If>
			<If cond={data.hasActiveRound}>
				<section class="portal-section">
					<header>
						<div>
							<p class="panel-kicker">{data.plan.instructions}</p>
							<h2>
								Your proposals (
								{data.submissionCount}
								)
							</h2>
						</div>
					</header>
					<If cond={data.submissionCount == 0}>
						<div class="empty-state">
							<strong>
								No proposals are assigned to you for this round yet.
							</strong>
							<p>
								The program team assigns proposals per round. Check back after assignment.
							</p>
						</div>
					</If>
					<div id="submissions" class="score-card-list">
						<Each of={data.submissions} as="submission">
							<SubmissionCard {...submission}></SubmissionCard>
						</Each>
					</div>
				</section>
			</If>
		</If>
	</main>
}
