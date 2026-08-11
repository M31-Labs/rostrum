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
				<ActionForm actionName="runDue">
					<input type="hidden" name="csrf_token" value={csrf.token}></input>
					<input type="hidden" name="selected_template" value={data.preview.id}></input>
					<p class="form-status" role="status" aria-live="polite">{actions.runDue.message}</p>
					<button class="button" type="submit">Run outbox now</button>
				</ActionForm>
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
				<span>Needs retry</span>
				<strong>{data.counts.failed}</strong>
			</article>
			<article>
				<span>Suppressed</span>
				<strong>{data.counts.suppressed}</strong>
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
						href={"/organizer/communications?template=" + template.id + "&recipient=" + data.preview.recipientId}
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
				<header>
					<p class="panel-kicker">Preview as</p>
					<h2>Recipients</h2>
				</header>
				<Each of={data.recipients} as="recipient">
					<a
						class={recipient.class}
						href={"/organizer/communications?template=" + data.preview.id + "&recipient=" + recipient.id}
						data-gosx-link
					>
						<div>
							<strong>{recipient.name}</strong>
							<small>{recipient.email}</small>
						</div>
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
					<p class="form-error" data-gosx-field-error="template_id" aria-live="polite"></p>
					<label>
						<span>Recipient</span>
						<select name="speaker_id">
							<Each of={data.recipients} as="recipient">
								<option value={recipient.id} selected={recipient.id == data.preview.recipientId}>
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
							<option value="configured">Configured transport — send now</option>
							<option value="gmail">Gmail — queue for connected account</option>
							<option value="outlook">Outlook — queue for connected account</option>
						</select>
						<p class="form-error" data-gosx-field-error="provider" aria-live="polite"></p>
					</label>
					<p class="form-status" role="status" aria-live="polite">{action.message}</p>
					<button class="button button-primary" type="submit">Queue message</button>
				</ActionForm>
				<section class="panel template-editor">
					<header class="panel-header">
						<div>
							<p class="panel-kicker">Editable source</p>
							<h3>Template editor</h3>
						</div>
						<If cond={data.preview.system}>
							<span class="status-pill status-neutral">Core template</span>
						</If>
					</header>
					<ActionForm class="settings-form" actionName="saveTemplate">
						<input type="hidden" name="csrf_token" value={csrf.token}></input>
						<input type="hidden" name="template_id" value={data.preview.id}></input>
						<div class="form-grid-two">
							<label>
								<span>Name</span>
								<input name="name" value={data.preview.name} disabled={data.preview.system}></input>
							</label>
							<label>
								<span>Audience</span>
								<select name="audience" disabled={data.preview.system}>
									<option value="speaker" selected={data.preview.audience == "speaker"}>Speaker</option>
									<option value="submitter" selected={data.preview.audience == "submitter"}>Submitter</option>
									<option value="administrator" selected={data.preview.audience == "administrator"}>Administrator</option>
								</select>
							</label>
						</div>
						<label>
							<span>Subject</span>
							<input name="subject" maxlength="240" value={data.preview.subject} required></input>
						</label>
						<label>
							<span>Message</span>
							<textarea name="body" maxlength="20000" required>{data.preview.bodySource}</textarea>
						</label>
						<label>
							<span>Reply-to</span>
							<input type="email" name="reply_to" value={data.preview.replyTo}></input>
						</label>
						<label class="checkbox-control">
							<input type="checkbox" name="attach_calendar" checked={data.preview.calendar}></input>
							<span>Attach a calendar invite when this recipient has a scheduled session</span>
						</label>
						<p class="form-error" data-gosx-field-error="template" aria-live="polite"></p>
						<p class="form-status" role="status" aria-live="polite">{actions.saveTemplate.message}</p>
						<button class="button button-primary" type="submit">Save revision</button>
					</ActionForm>
					<If cond={!data.preview.system}>
						<ActionForm class="field-remove-form" actionName="deleteTemplate">
							<input type="hidden" name="csrf_token" value={csrf.token}></input>
							<input type="hidden" name="template_id" value={data.preview.id}></input>
							<p class="form-status" role="status" aria-live="polite">{actions.deleteTemplate.message}</p>
							<button class="button" type="submit">Delete unused template</button>
						</ActionForm>
					</If>
					<If cond={data.revisions.length > 0}>
						<div class="template-revision-list">
							<strong>Recorded revisions</strong>
							<Each of={data.revisions} as="revision">
								<p>
									Revision {revision.revision} · {revision.when} · {revision.actor}
									<small>{revision.subject}</small>
								</p>
							</Each>
						</div>
					</If>
				</section>
			</section>
		</div>
		<section class="panel template-create-panel">
			<header class="panel-header">
				<div>
					<p class="panel-kicker">Template library</p>
					<h2>Create a template</h2>
				</div>
			</header>
			<ActionForm class="settings-form" actionName="createTemplate">
				<input type="hidden" name="csrf_token" value={csrf.token}></input>
				<div class="form-grid-two">
					<label><span>Name</span><input name="name" required placeholder="Final slides reminder"></input></label>
					<label>
						<span>Audience</span>
						<select name="audience"><option value="speaker">Speaker</option><option value="submitter">Submitter</option><option value="administrator">Administrator</option></select>
					</label>
				</div>
				<label><span>Subject</span><input name="subject" maxlength="240" required></input></label>
				<label><span>Message</span><textarea name="body" maxlength="20000" required></textarea></label>
				<label><span>Reply-to</span><input type="email" name="reply_to"></input></label>
				<label class="checkbox-control"><input type="checkbox" name="attach_calendar"></input><span>Attach a calendar invite when applicable</span></label>
				<p class="form-error" data-gosx-field-error="template" aria-live="polite"></p>
				<p class="form-status" role="status" aria-live="polite">{actions.createTemplate.message}</p>
				<button class="button" type="submit">Create template</button>
			</ActionForm>
			<p class="form-note">Supported merge fields: <Each of={data.mergeFields} as="field"><code>{field}</code> </Each></p>
		</section>
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
						<If cond={item.canCancel}>
							<ActionForm class="field-remove-form" actionName="cancelCommunication">
								<input type="hidden" name="csrf_token" value={csrf.token}></input>
								<input type="hidden" name="communication_id" value={item.id}></input>
								<input type="hidden" name="selected_template" value={data.preview.id}></input>
								<p class="form-status" role="status" aria-live="polite">{actions.cancelCommunication.message}</p>
								<button class="button button-compact" type="submit">Cancel</button>
							</ActionForm>
						</If>
					</article>
				</Each>
			</div>
		</section>
		<section class="panel notification-rule-panel">
			<header class="panel-header">
				<div>
					<p class="panel-kicker">Operations alerts</p>
					<h2>Administrator notification rules</h2>
				</div>
				<p>Triggers queue durable mail with retries and visible suppression decisions.</p>
			</header>
			<div class="notification-rule-list">
				<Each of={data.rules} as="rule">
					<ActionForm class="settings-form" actionName="saveNotificationRule">
						<input type="hidden" name="csrf_token" value={csrf.token}></input>
						<input type="hidden" name="rule_id" value={rule.id}></input>
						<input type="hidden" name="selected_template" value={data.preview.id}></input>
						<div class="form-grid-two">
							<label><span>Name</span><input name="name" value={rule.name} required></input></label>
							<label>
								<span>Trigger</span>
								<select name="trigger">
									<option value="submission.created" selected={rule.trigger == "submission.created"}>New proposal</option>
									<option value="submission.withdrawn" selected={rule.trigger == "submission.withdrawn"}>Proposal withdrawn</option>
									<option value="task.submitted" selected={rule.trigger == "task.submitted"}>Task submitted</option>
									<option value="task.approved" selected={rule.trigger == "task.approved"}>Task approved</option>
								</select>
							</label>
						</div>
						<label>
							<span>Administrator template</span>
							<select name="template_id">
								<Each of={data.templates} as="template">
									<If cond={template.audienceID == "administrator"}>
										<option value={template.id} selected={template.id == rule.templateID}>{template.name}</option>
									</If>
								</Each>
							</select>
						</label>
						<label><span>Recipients (comma-separated)</span><input type="text" name="recipients" value={rule.recipients} required></input></label>
						<div class="form-grid-two">
							<label><span>Attempts</span><input type="number" name="retry_limit" min="1" max="10" value={rule.retryLimit}></input></label>
							<label><span>Suppress duplicates (minutes)</span><input type="number" name="suppress_minutes" min="0" value={rule.suppressMinutes}></input></label>
						</div>
						<label class="checkbox-control"><input type="checkbox" name="enabled" checked={rule.enabled}></input><span>Enabled</span></label>
						<p class="form-error" data-gosx-field-error="rule" aria-live="polite"></p>
						<p class="form-status" role="status" aria-live="polite">{actions.saveNotificationRule.message}</p>
						<button class="button" type="submit">Save rule</button>
					</ActionForm>
					<ActionForm class="field-remove-form" actionName="removeNotificationRule">
						<input type="hidden" name="csrf_token" value={csrf.token}></input>
						<input type="hidden" name="rule_id" value={rule.id}></input>
						<input type="hidden" name="selected_template" value={data.preview.id}></input>
						<p class="form-status" role="status" aria-live="polite">{actions.removeNotificationRule.message}</p>
						<button class="button button-compact" type="submit">Remove rule</button>
					</ActionForm>
				</Each>
			</div>
			<ActionForm class="settings-form" actionName="saveNotificationRule">
				<input type="hidden" name="csrf_token" value={csrf.token}></input>
				<input type="hidden" name="selected_template" value={data.preview.id}></input>
				<h3>Add notification rule</h3>
				<label><span>Name</span><input name="name" required placeholder="Notify program team about task submissions"></input></label>
				<label>
					<span>Trigger</span>
					<select name="trigger"><option value="submission.created">New proposal</option><option value="submission.withdrawn">Proposal withdrawn</option><option value="task.submitted">Task submitted</option><option value="task.approved">Task approved</option></select>
				</label>
				<label>
					<span>Administrator template</span>
					<select name="template_id"><Each of={data.templates} as="template"><If cond={template.audienceID == "administrator"}><option value={template.id}>{template.name}</option></If></Each></select>
				</label>
				<label><span>Recipients</span><input name="recipients" required placeholder="program@example.com"></input></label>
				<div class="form-grid-two"><label><span>Attempts</span><input type="number" name="retry_limit" value="5" min="1" max="10"></input></label><label><span>Suppress duplicates (minutes)</span><input type="number" name="suppress_minutes" value="10" min="0"></input></label></div>
				<label class="checkbox-control"><input type="checkbox" name="enabled" checked></input><span>Enabled</span></label>
				<p class="form-status" role="status" aria-live="polite">{actions.saveNotificationRule.message}</p>
				<button class="button button-primary" type="submit">Add rule</button>
			</ActionForm>
		</section>
	</main>
}
