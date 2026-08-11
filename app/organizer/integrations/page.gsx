package integrations

func Page() Node {
	return <main id="main-content" class="workspace-page" data-section={data.section}>
		<header class="workspace-header">
			<div>
				<p class="eyebrow">One-way publishing</p>
				<h1>Integrations</h1>
				<p>
					Rostrum stays canonical. External event platforms receive deliberate, observable exports.
				</p>
			</div>
			<div class="workspace-header-actions">
				<ActionForm actionName="dryRun">
					<input type="hidden" name="csrf_token" value={csrf.token}></input>
					<p class="form-status" role="status" aria-live="polite">{action.message}</p>
					<button class="button" type="submit">Run Accelevents dry run</button>
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
					Publishes speaker profiles first, then scheduled sessions. Updates never pull data back into Rostrum.
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
		<section class="integration-hero airtable-integration">
			<div class="integration-logo airtable-logo" aria-hidden="true">AT</div>
			<div class="integration-copy">
				<p class="panel-kicker">Operational projection</p>
				<h2>{data.airtable.name}</h2>
				<p>
					Airtable receives idempotent speaker and scheduled-session records from Rostrum’s durable outbox. It never writes back into the program.
				</p>
				<div class="integration-facts">
					<span>
						<strong>{data.airtable.speakerCount}</strong>
						speakers
					</span>
					<span>
						<strong>{data.airtable.sessionCount}</strong>
						sessions
					</span>
					<span>
						<strong>{data.airtable.pending}</strong>
						queued
					</span>
				</div>
				<div class="integration-actions">
					<ActionForm actionName="airtableDryRun">
						<input type="hidden" name="csrf_token" value={csrf.token}></input>
						<p class="form-status" role="status" aria-live="polite">{actions.airtableDryRun.message}</p>
						<button class="button" type="submit">Run Airtable dry run</button>
					</ActionForm>
					<If cond={data.airtable.configured}>
						<ActionForm actionName="airtableSync">
							<input type="hidden" name="csrf_token" value={csrf.token}></input>
							<p class="form-status" role="alert" aria-live="assertive">{actions.airtableSync.message}</p>
							<button class="button button-primary" type="submit">Sync Airtable now</button>
						</ActionForm>
					</If>
					<If cond={!data.airtable.configured}>
						<button
							class="button button-primary"
							type="button"
							disabled
							title="Configure AIRTABLE_PAT and AIRTABLE_BASE_ID to unlock projection"
						>Live sync locked</button>
					</If>
				</div>
			</div>
			<div class="connection-state">
				<span class={"status-pill " + data.airtable.credentialTone}>{data.airtable.credentialLabel}</span>
				<small>{data.airtable.status}</small>
				<If cond={data.airtable.failed > 0}>
					<small>
						{data.airtable.failed}
						records are waiting for retry backoff.
					</small>
				</If>
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
			<section class="panel mapping-panel">
				<header class="panel-header">
					<div>
						<p class="panel-kicker">Airtable contract</p>
						<h2>Stable upserts</h2>
					</div>
					<span class="mono">PATCH × 10</span>
				</header>
				<div class="mapping-list">
					<article>
						<strong>{data.airtable.speakerTable}</strong>
						<code>
							Rostrum ID · Name · Email · Role · Company
						</code>
						<p>
							Speaker records are merged on
							<code>Rostrum ID</code>
							.
						</p>
					</article>
					<article>
						<strong>{data.airtable.sessionTable}</strong>
						<code>
							Rostrum ID · Title · Starts At · Ends At · Room · Track
						</code>
						<p>
							Only scheduled, non-cancelled sessions project. File uploads stay in operator-owned storage rather than becoming Airtable attachments.
						</p>
					</article>
				</div>
				<details class="payload-sample">
					<summary>Inspect Airtable record</summary>
					<p class="payload-code">{data.airtableSampleJSON}</p>
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
						Require explicit connected state for live publishing or projection.
					</li>
					<li>
						Stop on the first remote error; retain the ledger and retryable outbox.
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
							<strong>
								{run.integration}
								·
								{run.mode}
							</strong>
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
