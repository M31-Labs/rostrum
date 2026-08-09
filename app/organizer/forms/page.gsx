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
							<span class="drag-handle" aria-hidden="true">⠿</span>
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
					</Each>
				</div>
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
					<label class="field-control">
						<span>Close at</span>
						<input type="datetime-local" value={data.form.closeISO} disabled></input>
					</label>
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
