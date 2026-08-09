package integrations

func Page() Node {
	return <main id="main-content" class="workspace-page" data-section={data.section}>
		<header class="workspace-header">
			<div>
				<p class="eyebrow">One-way publishing</p>
				<h1>Integrations</h1>
				<p>
					Programma stays canonical. External event platforms receive deliberate, observable exports.
				</p>
			</div>
			<div class="workspace-header-actions">
				<ActionForm actionName="dryRun">
					<input type="hidden" name="csrf_token" value={csrf.token}></input>
					<p class="form-status" role="status" aria-live="polite">{action.message}</p>
					<button class="button" type="submit">Run dry run</button>
				</ActionForm>
				<If cond={data.integration.configured}>
					<ActionForm actionName="liveSync">
						<input type="hidden" name="csrf_token" value={csrf.token}></input>
						<p class="form-status" role="alert" aria-live="assertive">{actions.liveSync.message}</p>
						<button class="button button-primary" type="submit">Publish to Accelevents</button>
					</ActionForm>
				</If>
				<If cond={!data.integration.configured}>
					<button
						class="button button-primary"
						type="button"
						disabled
						title="Configure ACCELEVENTS_API_KEY to unlock live publishing"
					>Live publish locked</button>
				</If>
			</div>
		</header>
		<p class="flash-message">{flash.notice}</p>
		<section class="integration-hero">
			<div class="integration-logo" aria-hidden="true">A</div>
			<div class="integration-copy">
				<p class="panel-kicker">Native connector</p>
				<h2>{data.integration.name}</h2>
				<p>
					Publishes speaker profiles first, then scheduled sessions. Updates never pull data back into Programma.
				</p>
				<div class="integration-facts">
					<span>
						<strong>{data.integration.speakerCount}</strong>
						speakers
					</span>
					<span>
						<strong>{data.integration.sessionCount}</strong>
						sessions
					</span>
					<span>
						<strong>{data.integration.direction}</strong>
						direction
					</span>
				</div>
			</div>
			<div class="connection-state">
				<span class={"status-pill " + data.integration.credentialTone}>{data.integration.credentialLabel}</span>
				<small>{data.integration.status}</small>
			</div>
		</section>
		<div class="integration-grid">
			<section class="panel mapping-panel">
				<header class="panel-header">
					<div>
						<p class="panel-kicker">Payload contract</p>
						<h2>Field mapping</h2>
					</div>
					<span class="mono">POST JSON</span>
				</header>
				<div class="mapping-list">
					<article>
						<strong>Speaker profile</strong>
						<code>{data.integration.speakerEndpoint}</code>
						<p>
							Name, email, role, company, biography, links, and stable external ID.
						</p>
					</article>
					<article>
						<strong>Scheduled session</strong>
						<code>{data.integration.sessionEndpoint}</code>
						<p>
							Title, description, UTC times, location, track, and speaker external IDs.
						</p>
					</article>
				</div>
				<details class="payload-sample">
					<summary>Inspect sample payload</summary>
					<p class="payload-code">{data.sampleJSON}</p>
				</details>
			</section>
			<aside class="panel safety-panel">
				<header class="panel-header">
					<div>
						<p class="panel-kicker">Safety model</p>
						<h2>Fail closed</h2>
					</div>
				</header>
				<ol>
					<li>
						Build payloads from a validated snapshot.
					</li>
					<li>
						Dry run without credentials or network access.
					</li>
					<li>
						Require explicit connected state for live publishing.
					</li>
					<li>
						Stop on the first remote error and retain the ledger.
					</li>
				</ol>
				<p>
					Credentials come from the environment and never enter workspace JSON.
				</p>
			</aside>
		</div>
		<section class="panel sync-ledger">
			<header class="panel-header">
				<div>
					<p class="panel-kicker">Observable operations</p>
					<h2>Sync ledger</h2>
				</div>
				<span>
					{data.runCount}
					runs
				</span>
			</header>
			<div>
				<Each of={data.runs} as="run">
					<article>
						<span class={"status-pill status-" + run.tone}>{run.status}</span>
						<div>
							<strong>{run.mode}</strong>
							<small>{run.summary}</small>
						</div>
						<code>
							{run.speakers}
							speakers ·
							{run.sessions}
							sessions
						</code>
						<time>{run.when}</time>
					</article>
				</Each>
			</div>
		</section>
	</main>
}
