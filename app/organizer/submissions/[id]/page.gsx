package submissions

func NotFoundState() Node {
	return <section class="panel empty-state">
		<strong>Submission not found.</strong>
		<p>
			This proposal may have been removed. Return to the list and try again.
		</p>
		<a class="button" href="/organizer/submissions" data-gosx-link>Back to submissions</a>
	</section>
}

// AcceptForm is the detail page's dedicated one-click accept control. It
// posts the same "updateStatus" action the submissions list already uses
// (SP-1's bridge), so accepting here creates the unscheduled session exactly
// as accepting from the list does; the handler is not duplicated.
func AcceptForm(props any) Node {
	return <ActionForm class="accept-form" actionName="updateStatus">
		<input type="hidden" name="csrf_token" value={csrf.token}></input>
		<input type="hidden" name="submission_id" value={props.id}></input>
		<input type="hidden" name="status" value="accepted"></input>
		<p class="form-status" role="status" aria-live="polite">{action.message}</p>
		<button class="button button-primary" type="submit">Accept proposal</button>
	</ActionForm>
}

func StatusForm(props any) Node {
	return <ActionForm class="inline-status-form" actionName="updateStatus">
		<input type="hidden" name="csrf_token" value={csrf.token}></input>
		<input type="hidden" name="submission_id" value={props.id}></input>
		<label>
			<span>Change status</span>
			<select name="status" data-gosx-submit-on="change">
				<Each of={data.statusOptions} as="option">
					<option value={option.value} selected={option.value == props.status}>{option.label}</option>
				</Each>
			</select>
			<p class="form-error" data-gosx-field-error="status" aria-live="polite"></p>
		</label>
		<p class="form-status" role="status" aria-live="polite">{action.message}</p>
		<button class="button button-compact" type="submit">Update status</button>
	</ActionForm>
}

func AnswerRow(props any) Node {
	return <fragment>
		<dt>{props.label}</dt>
		<dd>{props.value}</dd>
	</fragment>
}

func SpeakerRow(props any) Node {
	return <article class="detail-speaker-row">
		<span class="avatar avatar-small">{props.initials}</span>
		<div>
			<strong>{props.name}</strong>
			<small>
				{props.role}
				·
				{props.company}
			</small>
		</div>
		<a class="button button-compact" href={props.portalURL} data-gosx-link>Open portal</a>
	</article>
}

func EvaluationRow(props any) Node {
	return <article class="detail-evaluation-row">
		<header>
			<div>
				<strong>{props.planName}</strong>
				<small>
					Reviewed by
					{props.reviewer}
					·
					{props.updated}
				</small>
			</div>
			<If cond={props.isAI}>
				<span class="ai-complete">
					AI ·
					{props.source}
					{props.model}
				</span>
			</If>
		</header>
		<div class="detail-evaluation-scores">
			<Each of={props.scores} as="score">
				<span>
					<strong>{score.value}</strong>
					/
					{score.max}
					{score.name}
				</span>
			</Each>
		</div>
		<p>
			<span class={"status-pill status-" + (props.isAI ? "accent" : "neutral")}>{props.recommendation}</span>
		</p>
		<p>{props.comments}</p>
	</article>
}

func Page() Node {
	return <main id="main-content" class="workspace-page" data-section={data.section}>
		<If cond={!data.found}>
			<header class="workspace-header">
				<div>
					<p class="eyebrow">Submission detail</p>
					<h1>Not found</h1>
				</div>
			</header>
			<NotFoundState></NotFoundState>
		</If>
		<If cond={data.found}>
			<header class="workspace-header">
				<div>
					<p class="eyebrow">Submission detail</p>
					<h1>{data.title}</h1>
					<p>
						<span class={"status-pill status-" + data.tone}>{data.status}</span>
						{data.category}
						·
						{data.format}
						·
						{data.level}
						· submitted
						{data.submitted}
					</p>
				</div>
				<div class="workspace-header-actions">
					<a class="button" href="/organizer/submissions" data-gosx-link>Back to submissions</a>
					<a class="button" href="/organizer/review#candidates" data-gosx-link>Open in review</a>
				</div>
			</header>
			<p class="flash-message">{flash.notice}</p>
			<div class="portal-grid">
				<div class="portal-primary">
					<section class="panel">
						<header class="panel-header">
							<div>
								<p class="panel-kicker">What the speaker proposed</p>
								<h2>Abstract</h2>
							</div>
						</header>
						<p class="detail-abstract">{data.abstract}</p>
					</section>
					<section class="panel">
						<header class="panel-header">
							<div>
								<p class="panel-kicker">Every field the speaker answered</p>
								<h2>Answers</h2>
							</div>
						</header>
						<dl class="detail-answers">
							<Each of={data.answers} as="answer">
								<AnswerRow label={answer.label} value={answer.value}></AnswerRow>
							</Each>
						</dl>
						<If cond={data.answerCount == 0}>
							<p>
								No additional answers were collected for this proposal.
							</p>
						</If>
					</section>
					<section class="panel">
						<header class="panel-header">
							<div>
								<p class="panel-kicker">Evaluation history</p>
								<h2>
									Evaluations (
									{data.evaluationCount}
									)
								</h2>
							</div>
							<span class="ai-legend">
								<i>AI</i>
								Second opinions are labeled
							</span>
						</header>
						<div class="detail-evaluation-list">
							<Each of={data.evaluations} as="evaluation">
								<EvaluationRow {...evaluation}></EvaluationRow>
							</Each>
						</div>
						<If cond={data.evaluationCount == 0}>
							<div class="empty-state">
								<strong>No evaluations yet.</strong>
								<p>
									Record a rubric review from the review workspace to start scoring this proposal.
								</p>
							</div>
						</If>
					</section>
				</div>
				<aside class="portal-secondary">
					<section class="panel">
						<header class="panel-header">
							<div>
								<p class="panel-kicker">Decision</p>
								<h2>Status</h2>
							</div>
						</header>
						<If cond={data.canAccept}>
							<AcceptForm id={data.id}></AcceptForm>
						</If>
						<StatusForm id={data.id} status={data.statusValue}></StatusForm>
					</section>
					<section class="panel">
						<header class="panel-header">
							<div>
								<p class="panel-kicker">Where it landed</p>
								<h2>Routing</h2>
							</div>
						</header>
						<dl class="detail-answers">
							<AnswerRow label="Queue" value={data.queue}></AnswerRow>
							<AnswerRow label="Owner" value={data.owner}></AnswerRow>
						</dl>
						<p class="panel-kicker">Rule trace</p>
						<ol class="detail-trace">
							<Each of={data.ruleTrace} as="step">
								<li>{step}</li>
							</Each>
						</ol>
					</section>
					<section class="panel">
						<header class="panel-header">
							<div>
								<p class="panel-kicker">
									{data.speakerCount}
									listed
								</p>
								<h2>Speakers</h2>
							</div>
						</header>
						<Each of={data.speakers} as="speaker">
							<SpeakerRow {...speaker}></SpeakerRow>
						</Each>
					</section>
					<If cond={data.hasSession}>
						<section class="panel">
							<header class="panel-header">
								<div>
									<p class="panel-kicker">Program</p>
									<h2>Session</h2>
								</div>
							</header>
							<p>
								<span class={"status-pill status-" + data.session.tone}>{data.session.status}</span>
								<strong>{data.session.title}</strong>
							</p>
							<p>{data.session.when}</p>
							<a class="button button-compact" href="/organizer/agenda" data-gosx-link>Open agenda</a>
						</section>
					</If>
					<If cond={!data.hasSession}>
						<section class="panel">
							<header class="panel-header">
								<div>
									<p class="panel-kicker">Program</p>
									<h2>Session</h2>
								</div>
							</header>
							<p>
								Not yet scheduled. Accepting this proposal creates an unscheduled session automatically.
							</p>
						</section>
					</If>
				</aside>
			</div>
		</If>
	</main>
}
