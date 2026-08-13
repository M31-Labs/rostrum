package portal

func Page() Node {
	return <main id="main-content" class="workspace-page" data-section={data.section}>
		<header class="workspace-header">
			<div>
				<p class="eyebrow">Speaker readiness</p>
				<h1>Portal & tasks</h1>
				<p>
					Outstanding work updates here as participants complete their profile, forms, and file requests.
				</p>
			</div>
			<div class="workspace-header-actions">
				<If cond={!data.workspace.readOnlyPreview && data.hasPreviewPortal}>
					<a class="button" href={data.previewPortalURL} data-gosx-link>Preview speaker portal</a>
				</If>
				<If cond={!data.workspace.readOnlyPreview}>
					<a class="button" href="/organizer/export/approved-uploads.zip">Approved uploads ZIP</a>
				</If>
				<If cond={data.workspace.readOnlyPreview}>
					<a class="button" href="/tour" data-gosx-link>Explore speaker persona</a>
				</If>
				<a class="button button-primary" href="/organizer/communications" data-gosx-link>Send reminder</a>
			</div>
		</header>
		<p class="flash-message">{flash.notice}</p>
		<section class="workspace-metric-grid portal-metrics">
			<Each of={data.metrics} as="metric">
				<article class="workspace-metric">
					<span>{metric.label}</span>
					<strong>{metric.value}</strong>
					<small>
						{metric.detail}
						% of assignments
					</small>
				</article>
			</Each>
		</section>
		<section class="panel task-dashboard" aria-live="polite" data-live-region>
			<header class="panel-header">
				<div>
					<p class="panel-kicker">Live completion matrix</p>
					<h2>Onboarding tasks</h2>
				</div>
				<span class="live-indicator">
					<i></i>
					Live
				</span>
			</header>
			<div class="task-table" role="table" aria-label="Speaker task completion">
				<div class="task-head" role="row">
					<span>Task</span>
					<span>Due</span>
					<span>Assigned</span>
					<span>Approved</span>
					<span>Submitted</span>
					<span>Outstanding</span>
					<span>Progress</span>
				</div>
				<Each of={data.tasks} as="task">
					<article class="task-row" role="row">
						<div>
							<strong>{task.title}</strong>
							<small>
								{task.type}
								·
								{task.description}
							</small>
						</div>
						<span class="mono">{task.due}</span>
						<strong>{task.assigned}</strong>
						<strong>{task.approved}</strong>
						<strong>{task.submitted}</strong>
						<strong class="critical-number">{task.outstanding}</strong>
						<div class="task-progress">
							<span>
								{task.percent}
								%
							</span>
							<div class="progress-track">
								<i class="progress-fill fill-positive" style={"--progress: " + task.percentStyle}></i>
							</div>
						</div>
					</article>
				</Each>
			</div>
		</section>
		<section id="task-manager" class="panel task-manager-panel">
			<header class="panel-header">
				<div>
					<p class="panel-kicker">Task operations</p>
					<h2>Create, assign, and retire speaker work</h2>
				</div>
				<span>
					{data.retiredCount}
					archived
				</span>
			</header>
			<p>
				Changes appear in assigned speaker portals immediately. Retiring a task preserves every submitted response and file for audit and export.
			</p>
			<If cond={!data.workspace.readOnlyPreview}>
				<ActionForm class="task-create-form" actionName="createTask">
					<input type="hidden" name="csrf_token" value={csrf.token}></input>
					<p class="form-status" role="status" aria-live="polite">{actions.createTask.message}</p>
					<div class="form-grid-two">
						<label>
							<span>Task title</span>
							<input name="title" maxlength="160" required placeholder="e.g. Submit final slides"></input>
							<p class="form-error" data-gosx-field-error="title" aria-live="polite"></p>
						</label>
						<label>
							<span>Delivery type</span>
							<select name="type" required>
								<option value="form">Confirmation or form</option>
								<option value="file">File upload</option>
								<option value="headshot">Headshot upload</option>
								<option value="profile">Profile update</option>
							</select>
							<p class="form-error" data-gosx-field-error="type" aria-live="polite"></p>
						</label>
					</div>
					<label>
						<span>Speaker instructions</span>
						<textarea
							name="description"
							maxlength="2000"
							placeholder="What is needed, and how will the team use it?"
						></textarea>
						<p class="form-error" data-gosx-field-error="description" aria-live="polite"></p>
					</label>
					<div class="form-grid-two">
						<label>
							<span>Due date</span>
							<input type="datetime-local" name="due_at" required></input>
							<p class="form-error" data-gosx-field-error="due_at" aria-live="polite"></p>
						</label>
						<div class="checkbox-stack">
							<label class="checkbox-control">
								<input type="checkbox" name="required" checked></input>
								<span>Required for readiness</span>
							</label>
							<label class="checkbox-control">
								<input type="checkbox" name="accepted_only" checked></input>
								<span>Only accepted speakers can receive it</span>
							</label>
							<label class="checkbox-control">
								<input type="checkbox" name="assign_all_accepted"></input>
								<span>Assign all currently accepted speakers</span>
							</label>
						</div>
					</div>
					<button class="button button-primary" type="submit">Create task</button>
				</ActionForm>
			</If>
			<If cond={data.hasTasks}>
				<div class="task-manager-list">
					<Each of={data.tasks} as="task">
						<article class="task-manager-card">
							<header>
								<div>
									<strong>{task.title}</strong>
									<small>
										{task.assigned}
										current assignment(s) ·
										{task.assignedNames}
									</small>
								</div>
								<span class="status-pill status-neutral">{task.type}</span>
							</header>
							<If cond={!data.workspace.readOnlyPreview}>
								<ActionForm class="task-update-form" actionName="updateTask">
									<input type="hidden" name="csrf_token" value={csrf.token}></input>
									<input type="hidden" name="task_id" value={task.id}></input>
									<p class="form-status" role="status" aria-live="polite">{actions.updateTask.message}</p>
									<div class="form-grid-two">
										<label>
											<span>Title</span>
											<input name="title" maxlength="160" value={task.title} required></input>
											<p class="form-error" data-gosx-field-error="title" aria-live="polite"></p>
										</label>
										<label>
											<span>Type</span>
											<select name="type">
												<option value="form" selected={task.typeValue == "form"}>Confirmation or form</option>
												<option value="file" selected={task.typeValue == "file"}>File upload</option>
												<option value="headshot" selected={task.typeValue == "headshot"}>Headshot upload</option>
												<option value="profile" selected={task.typeValue == "profile"}>Profile update</option>
											</select>
											<p class="form-error" data-gosx-field-error="type" aria-live="polite"></p>
										</label>
									</div>
									<label>
										<span>Instructions</span>
										<textarea name="description" maxlength="2000">{task.description}</textarea>
										<p class="form-error" data-gosx-field-error="description" aria-live="polite"></p>
									</label>
									<div class="form-grid-two">
										<label>
											<span>Due</span>
											<input type="datetime-local" name="due_at" value={task.dueInput} required></input>
											<p class="form-error" data-gosx-field-error="due_at" aria-live="polite"></p>
										</label>
										<div class="checkbox-stack">
											<label class="checkbox-control">
												<input type="checkbox" name="required" checked={task.required}></input>
												<span>Required</span>
											</label>
											<label class="checkbox-control">
												<input type="checkbox" name="accepted_only" checked={task.acceptedOnly}></input>
												<span>Accepted speakers only</span>
											</label>
										</div>
									</div>
									<button class="button button-compact" type="submit">Save task</button>
								</ActionForm>
								<div class="task-manager-actions">
									<ActionForm class="task-assign-form" actionName="assignTask">
										<input type="hidden" name="csrf_token" value={csrf.token}></input>
										<input type="hidden" name="task_id" value={task.id}></input>
										<label>
											<span>Assign speaker</span>
											<select name="speaker_id" required>
												<Each of={data.speakers} as="speaker">
													<option value={speaker.id} disabled={task.acceptedOnly && !speaker.accepted}>
														{speaker.name}
														<If cond={!speaker.accepted}>{" (not accepted)"}</If>
													</option>
												</Each>
											</select>
											<p class="form-error" data-gosx-field-error="speaker_id" aria-live="polite"></p>
										</label>
										<p class="form-status" role="status" aria-live="polite">{actions.assignTask.message}</p>
										<button class="button button-compact" type="submit">Assign</button>
									</ActionForm>
									<ActionForm class="task-retire-form" actionName="retireTask">
										<input type="hidden" name="csrf_token" value={csrf.token}></input>
										<input type="hidden" name="task_id" value={task.id}></input>
										<p class="form-status" role="status" aria-live="polite">{actions.retireTask.message}</p>
										<button class="button button-compact" type="submit">Retire task</button>
									</ActionForm>
								</div>
							</If>
						</article>
					</Each>
				</div>
			</If>
			<If cond={data.hasRetired}>
				<details class="task-archive-list">
					<summary>
						{data.retiredCount}
						retired task(s) retained for audit
					</summary>
					<Each of={data.retiredTasks} as="task">
						<p>
							<strong>{task.title}</strong>
							· retired
							{task.retiredAt}
						</p>
					</Each>
				</details>
			</If>
		</section>
		<section class="panel approval-queue" aria-labelledby="approval-queue-title">
			<header class="panel-header">
				<div>
					<p class="panel-kicker">Human checkpoint</p>
					<h2 id="approval-queue-title">Awaiting approval</h2>
				</div>
				<span class="status-pill status-accent">
					{data.approvalCount}
					ready
				</span>
			</header>
			<If cond={data.hasApprovals}>
				<div class="approval-list">
					<Each of={data.approvals} as="item">
						<article class="approval-item">
							<span class="avatar">{item.initials}</span>
							<div class="approval-copy">
								<strong>{item.taskTitle}</strong>
								<a href={item.portalURL} data-gosx-link>{item.speakerName}</a>
								<small>
									{item.taskType}
									· submitted
									{item.updated}
								</small>
								<If cond={item.hasFile && !data.workspace.readOnlyPreview}>
									<small class="mono">
										<a href={item.fileURL} class="file-download">{item.fileName}</a>
									</small>
								</If>
								<If cond={item.hasFile && data.workspace.readOnlyPreview}>
									<small class="mono">{item.fileName}</small>
								</If>
							</div>
							<If cond={!data.workspace.readOnlyPreview}>
								<ActionForm class="approval-action" actionName="approveTask">
									<input type="hidden" name="csrf_token" value={csrf.token}></input>
									<input type="hidden" name="task_id" value={item.taskID}></input>
									<input type="hidden" name="speaker_id" value={item.speakerID}></input>
									<p class="form-status" role="status" aria-live="polite">{action.message}</p>
									<button class="button button-primary button-compact" type="submit">Approve</button>
								</ActionForm>
							</If>
						</article>
					</Each>
				</div>
			</If>
			<If cond={!data.hasApprovals}>
				<div class="empty-state">
					<strong>Approval queue clear</strong>
					<p>
						New speaker submissions will appear here as soon as they arrive.
					</p>
				</div>
			</If>
		</section>
		<div class="two-column-workspace portal-lower-grid">
			<section class="panel">
				<header class="panel-header">
					<div>
						<p class="panel-kicker">Follow-up queue</p>
						<h2>People needing attention</h2>
					</div>
				</header>
				<div class="portal-people">
					<Each of={data.people} as="person">
						<article>
							<span class="avatar">{person.initials}</span>
							<div>
								<strong>{person.name}</strong>
								<small>
									{person.outstanding}
									outstanding ·
									{person.complete}
									/
									{person.total}
									complete
								</small>
							</div>
							<span class="mono">
								{person.percent}
								%
							</span>
							<a class="button button-compact" href={person.portalURL} data-gosx-link>Open</a>
						</article>
					</Each>
				</div>
			</section>
			<aside class="panel">
				<header class="panel-header">
					<div>
						<p class="panel-kicker">Portal wiki</p>
						<h2>Shared resources</h2>
					</div>
				</header>
				<div class="resource-admin-list">
					<Each of={data.resources} as="resource">
						<article>
							<span class="resource-order mono">
								0
								{resource.order}
							</span>
							<div>
								<strong>{resource.title}</strong>
								<small>
									{resource.kind}
									·
									{resource.summary}
								</small>
							</div>
						</article>
					</Each>
				</div>
			</aside>
		</div>
	</main>
}
