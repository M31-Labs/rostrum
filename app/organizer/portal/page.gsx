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
				<a class="button" href="/portal/spk_maya" data-gosx-link>Preview speaker portal</a>
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
								<If cond={item.hasFile}>
									<small class="mono">
										<a href={item.fileURL} class="file-download">{item.fileName}</a>
									</small>
								</If>
							</div>
							<ActionForm class="approval-action" actionName="approveTask">
								<input type="hidden" name="csrf_token" value={csrf.token}></input>
								<input type="hidden" name="task_id" value={item.taskID}></input>
								<input type="hidden" name="speaker_id" value={item.speakerID}></input>
								<p class="form-status" role="status" aria-live="polite">{action.message}</p>
								<button class="button button-primary button-compact" type="submit">Approve</button>
							</ActionForm>
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
