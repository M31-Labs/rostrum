package portal

func TaskAction(props any) Node {
	return <ActionForm class="portal-task-action" actionName="completeTask">
		<input type="hidden" name="csrf_token" value={csrf.token}></input>
		<input type="hidden" name="task_id" value={props.id}></input>
		<p class="form-error" data-gosx-field-error="task" aria-live="polite"></p>
		<If cond={props.kind == "form"}>
			<label class="checkbox-control">
				<input type="checkbox" name="confirmed" value="yes" required></input>
				<span>I confirm these details are complete.</span>
			</label>
		</If>
		<p class="form-status" role="status" aria-live="polite">{action.message}</p>
		<button class="button button-compact" type="submit">
			<If cond={props.complete}>Resubmit</If>
			<If cond={!props.complete}>Complete task</If>
		</button>
	</ActionForm>
}

func FileAction(props any) Node {
	return <Form
		class="portal-task-action file-action"
		method="post"
		enctype="multipart/form-data"
		action={"/portal-upload/" + data.speaker.id + "/" + props.id}
	>
		<input type="hidden" name="csrf_token" value={csrf.token}></input>
		<label>
			<span class="sr-only">
				Choose file for
				{props.title}
			</span>
			<input type="file" name="file" required></input>
		</label>
		<button class="button button-compact" type="submit">Upload file</button>
	</Form>
}

func ProfileFields(props any) Node {
	return <fragment>
		<div class="form-grid-two">
			<label>
				<span>Role</span>
				<input name="role" value={props.role}></input>
			</label>
			<label>
				<span>Company or project</span>
				<input name="company" value={props.company}></input>
			</label>
			<label>
				<span>Pronouns</span>
				<input name="pronouns" value={props.pronouns}></input>
			</label>
			<label>
				<span>City</span>
				<input name="city" value={props.city}></input>
			</label>
		</div>
		<label>
			<span>Biography</span>
			<textarea
				name="biography"
				maxlength="800"
				minlength="40"
				aria-describedby="profile-biography-error"
				required
			>{props.biography}</textarea>
			<p class="form-error" id="profile-biography-error" aria-live="polite">{props.biographyError}</p>
		</label>
		<div class="form-grid-two">
			<label>
				<span>LinkedIn URL</span>
				<input type="url" name="linkedin" value={props.linkedin}></input>
			</label>
			<label>
				<span>Website URL</span>
				<input type="url" name="website" value={props.website}></input>
			</label>
		</div>
	</fragment>
}

func PortalUnavailable(props any) Node {
	return <section class="portal-section portal-unavailable">
		<p class="panel-kicker">Speaker portal</p>
		<h1>Check your email for your portal link</h1>
		<p>
			This link is missing or no longer active. Open the portal link from your invitation email to continue, or contact the program team for a new one.
		</p>
	</section>
}

func Page() Node {
	return <main id="main-content" class="portal-main">
		<If cond={!data.available}>
			<PortalUnavailable></PortalUnavailable>
		</If>
		<If cond={data.available}>
			<If cond={data.submitted}>
				<div class="portal-confirmation">
					<span aria-hidden="true">✓</span>
					<div>
						<strong>Proposal received.</strong>
						<p>
							Your confirmation is in the demo outbox. You can finish your profile here while review begins.
						</p>
					</div>
				</div>
			</If>
			<p class="portal-flash">{flash.notice}</p>
			<section class="portal-hero">
				<div class="portal-profile-summary">
					<span class="avatar portal-avatar">{data.speaker.initials}</span>
					<div>
						<p class="eyebrow">Speaker portal</p>
						<h1>
							Welcome,
							{data.speaker.firstName}
							.
						</h1>
						<p>
							{data.event.name}
							·
							{data.event.dates}
							·
							{data.event.location}
						</p>
					</div>
				</div>
				<div class="readiness-dial">
					<strong>
						{data.readiness.percent}
						%
					</strong>
					<span>Ready</span>
					<div class="readiness-bar">
						<i style={"--progress: " + data.readiness.style}></i>
					</div>
					<small>
						{data.readiness.complete}
						of
						{data.readiness.total}
						tasks complete
					</small>
				</div>
			</section>
			<div class="portal-grid">
				<div class="portal-primary">
					<section id="tasks" class="portal-section">
						<header>
							<div>
								<p class="panel-kicker">Your checklist</p>
								<h2>Tasks</h2>
							</div>
							<span>
								{data.readiness.complete}
								/
								{data.readiness.total}
								complete
							</span>
						</header>
						<div class="portal-task-list">
							<Each of={data.tasks} as="task">
								<article class="portal-task">
									<div class="task-state">
										<span class={"status-pill status-" + task.tone}>{task.status}</span>
										<time>
											Due
											{task.due}
										</time>
									</div>
									<div>
										<h3>{task.title}</h3>
										<p>{task.description}</p>
										<If cond={task.fileName != ""}>
											<p class="uploaded-file">
												Current file ·
												{task.fileName}
											</p>
										</If>
									</div>
									<If cond={task.kind == "file"}>
										<FileAction {...task}></FileAction>
									</If>
									<If cond={task.kind != "file"}>
										<TaskAction {...task}></TaskAction>
									</If>
								</article>
							</Each>
						</div>
					</section>
					<section class="portal-section profile-editor">
						<header>
							<div>
								<p class="panel-kicker">Self-service profile</p>
								<h2>Public bio</h2>
							</div>
							<span>Updates sync to the gallery</span>
						</header>
						<ActionForm actionName="updateProfile">
							<input type="hidden" name="csrf_token" value={csrf.token}></input>
							<p class="form-status" role="alert" aria-live="assertive">{actions.updateProfile.message}</p>
							<If cond={actions.updateProfile.name != ""}>
								<ProfileFields
									role={actions.updateProfile.values.role}
									company={actions.updateProfile.values.company}
									pronouns={actions.updateProfile.values.pronouns}
									city={actions.updateProfile.values.city}
									biography={actions.updateProfile.values.biography}
									biographyError={actions.updateProfile.fieldErrors.biography}
									linkedin={actions.updateProfile.values.linkedin}
									website={actions.updateProfile.values.website}
								></ProfileFields>
							</If>
							<If cond={actions.updateProfile.name == ""}>
								<ProfileFields
									role={data.speaker.role}
									company={data.speaker.company}
									pronouns={data.speaker.pronouns}
									city={data.speaker.city}
									biography={data.speaker.biography}
									biographyError=""
									linkedin={data.speaker.linkedIn}
									website={data.speaker.website}
								></ProfileFields>
							</If>
							<button class="button button-primary" type="submit">Save public profile</button>
						</ActionForm>
					</section>
				</div>
				<aside class="portal-secondary">
					<section id="schedule" class="portal-section">
						<header>
							<div>
								<p class="panel-kicker">Your itinerary</p>
								<h2>Schedule</h2>
							</div>
							<a href={data.calendarURL}>Add all to calendar</a>
						</header>
						<div class="portal-sessions">
							<Each of={data.sessions} as="session">
								<article class={"track-rail track-" + session.trackTone}>
									<time>
										{session.date}
										<strong>{session.time}</strong>
									</time>
									<h3>{session.title}</h3>
									<p>
										{session.room}
										·
										{session.track}
									</p>
									<span class={"status-pill status-" + session.tone}>{session.status}</span>
								</article>
							</Each>
							<If cond={data.sessionCount == 0}>
								<p>
									Your session time will appear after program scheduling.
								</p>
							</If>
						</div>
					</section>
					<section class="portal-section">
						<header>
							<div>
								<p class="panel-kicker">Your proposals</p>
								<h2>Submissions</h2>
							</div>
						</header>
						<Each of={data.proposals} as="proposal">
							<article class="portal-proposal">
								<strong>{proposal.title}</strong>
								<span class={"status-pill status-" + proposal.tone}>{proposal.status}</span>
								<small>
									Submitted
									{proposal.submitted}
								</small>
							</article>
						</Each>
					</section>
				</aside>
			</div>
			<section id="resources" class="portal-section resource-library">
				<header>
					<div>
						<p class="panel-kicker">Program wiki</p>
						<h2>Resources</h2>
					</div>
					<span>Everything you need on event week</span>
				</header>
				<div class="resource-grid">
					<Each of={data.resources} as="resource">
						<article>
							<span class="resource-order mono">
								0
								{resource.order}
							</span>
							<p class="panel-kicker">{resource.kindLabel}</p>
							<h3>{resource.title}</h3>
							<p>{resource.summary}</p>
							<If cond={resource.kind == "article"}>
								<details>
									<summary>Read guide</summary>
									<p>{resource.body}</p>
								</details>
							</If>
							<If cond={resource.kind == "link"}>
								<a href={resource.url} target="_blank" rel="noreferrer">Open resource ↗</a>
							</If>
							<If cond={resource.kind == "embed"}>
								<div class="resource-embed">
									<iframe src={resource.embedURL} title={resource.title} loading="lazy" allowfullscreen></iframe>
								</div>
							</If>
							<If cond={resource.kind == "blocked"}>
								<p class="form-error">
									This embed origin is not on the portal allowlist.
								</p>
							</If>
						</article>
					</Each>
				</div>
			</section>
		</If>
	</main>
}
