package speakers

func Page() Node {
	return <main id="main-content" class="workspace-page" data-section={data.section}>
		<header class="workspace-header">
			<div>
				<p class="eyebrow">Participant operations</p>
				<h1>Speakers</h1>
				<p>
					{data.resultCount}
					shown of
					{data.totalCount}
					.
					{data.readyCount}
					have completed every assigned task.
				</p>
			</div>
			<div class="workspace-header-actions">
				<a class="button" href="/public/m31-systems-forum-2026/speakers" data-gosx-link>Preview gallery</a>
				<a class="button button-primary" href="/organizer/communications" data-gosx-link>Message cohort</a>
			</div>
		</header>
		<Form class="filter-bar speaker-filter" method="get" action="/organizer/speakers">
			<label class="search-control">
				<span class="sr-only">Search speakers</span>
				<input type="search" name="q" value={data.query} placeholder="Search name, company, or email"></input>
			</label>
			<button class="button button-primary" type="submit">Search</button>
			<a class="button button-compact" href="/organizer/speakers" data-gosx-link>Clear</a>
		</Form>
		<section class="speaker-grid">
			<Each of={data.rows} as="speaker">
				<article class="speaker-card">
					<header>
						<span class="avatar avatar-large">{speaker.initials}</span>
						<div>
							<h2>{speaker.name}</h2>
							<p>
								{speaker.role}
								·
								{speaker.company}
							</p>
						</div>
						<span class="readiness-number">
							{speaker.readiness}
							%
						</span>
					</header>
					<div class="readiness-bar">
						<span style={"--progress: " + speaker.readinessStyle}></span>
					</div>
					<div class="speaker-facts">
						<span>
							<strong>
								{speaker.taskCount - speaker.outstanding}
								/
								{speaker.taskCount}
							</strong>
							tasks
						</span>
						<span>
							<strong>{speaker.sessions}</strong>
							sessions
						</span>
						<span>
							<strong>
								{speaker.profileComplete}
								%
							</strong>
							profile
						</span>
					</div>
					<footer>
						<div>
							<strong>{speaker.readinessLabel}</strong>
							<small>
								{speaker.outstanding}
								outstanding · updated
								{speaker.updated}
							</small>
						</div>
						<a class="button button-compact" href={speaker.portalURL} data-gosx-link>Open portal</a>
					</footer>
				</article>
			</Each>
		</section>
		<If cond={data.resultCount == 0}>
			<div class="empty-state">
				<strong>No speakers match.</strong>
				<p>Clear the search and try again.</p>
			</div>
		</If>
	</main>
}
