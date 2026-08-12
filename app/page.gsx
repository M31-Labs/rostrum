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
				<p class="eyebrow">Program operations for real events</p>
				<h1>
					The calm way to run a complicated program.
				</h1>
				<p class="lede">
					Rostrum connects proposals, review, speakers, schedules, and public publishing without hiding decisions in a maze of automation.
				</p>
				<div class="hero-actions">
					<a class="button button-primary" href="/organizer" data-gosx-link>
						{data.demoMode ? "Enter the demo workspace" : "Enter the workspace"}
					</a>
					<a class="text-link" href={data.workspace.cfpHref} data-gosx-link hidden={!data.workspace.hasCFP}>
						Walk the speaker path
						<span aria-hidden="true">→</span>
					</a>
				</div>
			</div>
			<aside class="event-ticket" aria-label={data.demoMode ? "Demo event" : "Event"}>
				<div class="ticket-topline">
					<span>Live workspace</span>
					<span>{data.event.year}</span>
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
			<h2>Every program decision, on the record.</h2>
			<p>
				Routing, review, and scheduling each leave a visible trace. When someone asks why a talk landed where it did, the answer is one click away — not in someone’s inbox.
			</p>
			<div class="mission-links">
				<a href="/tour" data-gosx-link>Take the product tour</a>
				<a href="/organizer/agenda" data-gosx-link>See the scheduling rules</a>
				<a href="/organizer/forms" data-gosx-link>See the routing rules</a>
				<a href="/api/v1/workspace">Open the JSON API</a>
				<a href="https://m31-labs.github.io/rostrum/" target="_blank" rel="noreferrer">Read the field guide ↗</a>
				<a href="https://github.com/M31-Labs/rostrum" target="_blank" rel="noreferrer">Browse the source ↗</a>
			</div>
			<p class="colophon">
				Open-source, self-hostable, one binary. Built with GoSX.
			</p>
		</section>
	</main>
}
