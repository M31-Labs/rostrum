package organizer

func WorkspaceMetric(props any) Node {
	return <a class="workspace-metric" href={props.href} data-gosx-link>
		<span>{props.label}</span>
		<strong>{props.value}</strong>
		<small>{props.detail}</small>
	</a>
}

func StatusPill(props any) Node {
	return <span class={"status-pill status-" + props.tone}>{props.label}</span>
}

func Page() Node {
	return <main id="main-content" class="workspace-page" data-section={data.section}>
		<header class="workspace-header">
			<div>
				<p class="eyebrow">Command center</p>
				<h1>{data.event.name}</h1>
				<p>
					{data.event.dates}
					·
					{data.event.location}
				</p>
			</div>
			<div class="workspace-header-actions">
				<a class="button" href={data.workspace.publicAgendaHref} data-gosx-link>Preview event</a>
				<a class="button button-primary" href="/organizer/communications" data-gosx-link>Message speakers</a>
			</div>
		</header>
		<section class="workspace-metric-grid" aria-label="Program health">
			<Each of={data.metrics} as="metric">
				<WorkspaceMetric {...metric}></WorkspaceMetric>
			</Each>
		</section>
		<div class="dashboard-grid">
			<section class="panel panel-wide">
				<header class="panel-header">
					<div>
						<p class="panel-kicker">Intake</p>
						<h2>Submission pipeline</h2>
					</div>
					<a href="/organizer/submissions" data-gosx-link>Open board</a>
				</header>
				<div class="pipeline-list">
					<Each of={data.pipeline} as="stage">
						<div class="pipeline-row">
							<div class="pipeline-label">
								<span>{stage.label}</span>
								<strong>{stage.count}</strong>
							</div>
							<div class="progress-track">
								<span class={"progress-fill fill-" + stage.tone} style={"--progress: " + stage.width}></span>
							</div>
						</div>
					</Each>
				</div>
			</section>
			<section class="panel">
				<header class="panel-header">
					<div>
						<p class="panel-kicker">Needs attention</p>
						<h2>Before publish</h2>
					</div>
					<span class="live-indicator">
						<i></i>
						Live
					</span>
				</header>
				<div class="attention-list" aria-live="polite" data-live-region>
					<Each of={data.attention} as="item">
						<a class={"attention-item attention-" + item.tone} href={item.href} data-gosx-link>
							<strong>{item.value}</strong>
							<div>
								<span>{item.title}</span>
								<small>{item.detail}</small>
							</div>
							<b aria-hidden="true">→</b>
						</a>
					</Each>
				</div>
			</section>
		</div>
		<section class="panel schedule-preview">
			<header class="panel-header">
				<div>
					<p class="panel-kicker">{data.event.agendaDate}</p>
					<h2>Agenda pulse</h2>
				</div>
				<a href="/organizer/agenda" data-gosx-link>Open scheduler</a>
			</header>
			<div class="schedule-table" role="table" aria-label="Upcoming sessions">
				<div class="schedule-table-head" role="row">
					<span role="columnheader">Time</span>
					<span role="columnheader">Session</span>
					<span role="columnheader">Room / track</span>
					<span role="columnheader">Status</span>
				</div>
				<Each of={data.nextSessions} as="session">
					<div class="schedule-table-row" role="row">
						<span class="mono" role="cell">{session.time}</span>
						<div role="cell">
							<strong>{session.title}</strong>
							<small>{session.speakers}</small>
						</div>
						<div role="cell">
							<span>{session.room}</span>
							<small>{session.track}</small>
						</div>
						<span role="cell">
							<StatusPill label={session.status} tone={session.tone}></StatusPill>
						</span>
					</div>
				</Each>
			</div>
		</section>
		<p class="workspace-update">
			Workspace snapshot
			{data.live.updated}
		</p>
	</main>
}
