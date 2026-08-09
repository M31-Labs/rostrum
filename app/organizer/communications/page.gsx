package communications

func Page() Node {
	return <main id="main-content" class="workspace-page" data-section={data.section}>
		<header class="workspace-header">
			<div>
				<p class="eyebrow">Coordinated outreach</p>
				<h1>Communications</h1>
				<p>
					Reusable templates, scheduled reminders, provider handoff, and standards-based calendar attachments.
				</p>
			</div>
			<div class="workspace-header-actions">
				<a class="button" href={data.preview.gmailURL} target="_blank" rel="noreferrer">Open in Gmail</a>
				<a class="button" href={data.preview.outlookURL} target="_blank" rel="noreferrer">Open in Outlook</a>
				<a class="button button-primary" href={data.preview.calendarURL}>Download iCal</a>
			</div>
		</header>
		<p class="flash-message">{flash.notice}</p>
		<section class="communication-stats">
			<article>
				<span>Templates</span>
				<strong>{data.counts.templates}</strong>
			</article>
			<article>
				<span>Sent</span>
				<strong>{data.counts.sent}</strong>
			</article>
			<article>
				<span>Queued</span>
				<strong>{data.counts.queued}</strong>
			</article>
			<article>
				<span>Calendar format</span>
				<strong class="mono">RFC 5545</strong>
			</article>
		</section>
		<div class="communication-grid">
			<aside class="template-list">
				<header>
					<p class="panel-kicker">Library</p>
					<h2>Templates</h2>
				</header>
				<Each of={data.templates} as="template">
					<a
						class={template.class}
						href={"/organizer/communications?template=" + template.id}
						data-gosx-link
					>
						<div>
							<strong>{template.name}</strong>
							<small>{template.audience}</small>
						</div>
						<If cond={template.calendar}>
							<span title="Calendar attached">iCal</span>
						</If>
					</a>
				</Each>
			</aside>
			<section class="message-preview">
				<header>
					<div>
						<p class="panel-kicker">Live merge preview</p>
						<h2>{data.preview.name}</h2>
					</div>
					<span class="status-pill status-positive">Ready</span>
				</header>
				<div class="email-chrome">
					<div>
						<span>To</span>
						<strong>
							{data.preview.speaker}
							&lt;
							{data.preview.email}
							&gt;
						</strong>
					</div>
					<div>
						<span>Reply-to</span>
						<strong>{data.preview.replyTo}</strong>
					</div>
					<div>
						<span>Subject</span>
						<strong>{data.preview.subject}</strong>
					</div>
				</div>
				<article class="email-body">{data.preview.body}</article>
				<If cond={data.preview.calendar}>
					<div class="calendar-attachment">
						<span aria-hidden="true">15</span>
						<div>
							<strong>Calendar invitation attached</strong>
							<small>
								Google Calendar, Outlook, and Apple Calendar compatible
							</small>
						</div>
						<a href={data.preview.calendarURL}>Download .ics</a>
					</div>
				</If>
				<ActionForm class="message-send-form" actionName="queueMessage">
					<input type="hidden" name="csrf_token" value={csrf.token}></input>
					<input type="hidden" name="template_id" value={data.preview.id}></input>
					<label>
						<span>Recipient</span>
						<select name="speaker_id">
							<Each of={data.recipients} as="recipient">
								<option value={recipient.id}>
									{recipient.name}
									·
									{recipient.email}
								</option>
							</Each>
						</select>
					</label>
					<label>
						<span>Delivery</span>
						<select name="provider">
							<option value="demo-outbox">Demo outbox — send now</option>
							<option value="gmail">Gmail — queue for connected account</option>
							<option value="outlook">Outlook — queue for connected account</option>
						</select>
						<p class="form-error" data-gosx-field-error="provider" aria-live="polite"></p>
					</label>
					<p class="form-status" role="status" aria-live="polite">{action.message}</p>
					<button class="button button-primary" type="submit">Queue message</button>
				</ActionForm>
			</section>
		</div>
		<section class="panel outbox-panel">
			<header class="panel-header">
				<div>
					<p class="panel-kicker">Delivery ledger</p>
					<h2>Outbox</h2>
				</div>
				<span>
					{data.counts.sent}
					sent ·
					{data.counts.queued}
					queued
				</span>
			</header>
			<div class="outbox-list">
				<Each of={data.outbox} as="item">
					<article>
						<div>
							<strong>{item.subject}</strong>
							<small>{item.speaker}</small>
						</div>
						<span class={"status-pill status-" + item.tone}>{item.status}</span>
						<code>{item.provider}</code>
						<time>{item.when}</time>
					</article>
				</Each>
			</div>
		</section>
	</main>
}
