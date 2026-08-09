package app

func Metric(props any) Node {
	return <article class="metric">
		<strong>{props.value}</strong>
		<div>
			<span>{props.label}</span>
			<small>{props.detail}</small>
		</div>
	</article>
}

func WorkflowLink(props any) Node {
	return <a class="surface" href={props.href} data-gosx-link>
		<span class="surface-number">{props.number}</span>
		<div>
			<h3>{props.title}</h3>
			<p>{props.body}</p>
		</div>
		<span class="surface-arrow" aria-hidden="true">↗</span>
	</a>
}

func Page() Node {
	return <main id="main-content" class="landing-shell">
		<section class="landing-hero">
			<div class="hero-rule" aria-hidden="true"></div>
			<div class="hero-copy">
				<p class="eyebrow">Open program operations / GoSX</p>
				<h1>
					The calm way to run a complicated program.
				</h1>
				<p class="lede">
					Programma connects proposals, review, speakers, schedules, and public publishing without hiding decisions in a maze of automation.
				</p>
				<div class="hero-actions">
					<a class="button button-primary" href="/organizer" data-gosx-link>Enter the demo workspace</a>
					<a class="text-link" href="/submit/systems-forum-cfp" data-gosx-link>
						Walk the speaker path
						<span aria-hidden="true">→</span>
					</a>
				</div>
			</div>
			<aside class="event-ticket" aria-label="Demo event">
				<div class="ticket-topline">
					<span>Live workspace</span>
					<span>2026</span>
				</div>
				<div class="ticket-orbit" aria-hidden="true">
					<span></span>
				</div>
				<div class="ticket-event">
					<strong>{data.event.name}</strong>
					<span>{data.event.dates}</span>
					<span>{data.event.location}</span>
				</div>
			</aside>
		</section>
		<section class="metric-strip" aria-label="Workspace summary">
			<Each of={data.stats} as="metric">
				<Metric {...metric}></Metric>
			</Each>
			<p class="live-note">
				<span class="live-dot" aria-hidden="true"></span>
				Seeded workspace updated
				{data.updated}
			</p>
		</section>
		<section class="landing-workflow">
			<header class="section-heading split-heading">
				<div>
					<p class="eyebrow">One source of operational truth</p>
					<h2>From open call to room-ready.</h2>
				</div>
				<p>
					Each handoff stays connected. Change a session once; the portal, calendar, public schedule, and export all use the same record.
				</p>
			</header>
			<div class="surface-list">
				<Each of={data.surfaces} as="surface">
					<WorkflowLink {...surface}></WorkflowLink>
				</Each>
			</div>
		</section>
		<section class="mission-band">
			<p class="eyebrow">Built in the open</p>
			<h2>
				GoSX renders the workflow. Arbiter explains the decisions.
			</h2>
			<p>
				This submission is also a working proof of M31 Labs’ mission: compact tools, governed behavior, understandable systems, and an open path to self-hosting.
			</p>
			<div class="mission-links">
				<a href="/organizer/agenda" data-gosx-link>Inspect conflict policy</a>
				<a href="/organizer/forms" data-gosx-link>Inspect CFP routing</a>
				<a href="/api/v1/workspace">Open JSON API</a>
			</div>
		</section>
	</main>
}
