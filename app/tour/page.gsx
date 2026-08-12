package tour

func TourStop(props any) Node {
	return <article class={"tour-stop tour-stop-" + props.tone}>
		<div class="tour-stop-folio">
			<span>{props.number}</span>
			<small>{props.role}</small>
		</div>
		<div class="tour-stop-copy">
			<h2>{props.label}</h2>
			<p>{props.body}</p>
		</div>
		<a class="tour-stop-link" href={props.href} data-gosx-link>
			{props.action}
			<span aria-hidden="true">↗</span>
		</a>
	</article>
}

func Page() Node {
	return <main id="main-content" class="tour-shell">
		<header class="tour-hero">
			<div class="tour-hero-copy">
				<p class="eyebrow">Five roles · one source of truth</p>
				<h1>Follow the program all the way through.</h1>
				<p class="lede">
					Rostrum is easiest to understand as a handoff: one proposal becomes a reviewed decision, a ready speaker, a safe room assignment, and a public session—without becoming five disconnected records.
				</p>
			</div>
			<aside class="tour-field-note" aria-label="Tour notes">
				<p class="panel-kicker">Field note 00</p>
				<strong>{data.eventName}</strong>
				<p hidden={!data.readOnlyDemo}>
					This hosted tour uses fictional data. Every role is inspectable; every mutation is blocked at both the HTTP and storage boundaries.
				</p>
				<p hidden={data.readOnlyDemo}>
					This workspace is interactive. Signed reviewer and speaker links are issued from their organizer pages, never exposed publicly.
				</p>
			</aside>
		</header>
		<section class="tour-runway" aria-label="Rostrum persona walkthrough">
			<Each of={data.personas} as="persona">
				<TourStop
					number={persona.number}
					role={persona.role}
					label={persona.label}
					body={persona.body}
					href={persona.href}
					action={persona.action}
					tone={persona.tone}
				></TourStop>
			</Each>
		</section>
		<section class="tour-proof">
			<div>
				<p class="eyebrow">Trust the artifact, then inspect it</p>
				<h2>
					The demo and the source tell the same story.
				</h2>
			</div>
			<p>
				The public API exposes only publishable records. Rule traces show why routing and scheduling decisions happened. The repository carries the tests, deployment boundary, performance budgets, and operator runbooks behind those claims.
			</p>
			<div class="tour-proof-links">
				<a class="button button-primary" href={data.sourceURL} target="_blank" rel="noreferrer">Browse the source ↗</a>
				<a class="text-link" href={data.docsURL} target="_blank" rel="noreferrer">Read the field guide ↗</a>
				<a class="text-link" href="/api/v1/workspace">
					Inspect the public API
					<span aria-hidden="true">→</span>
				</a>
			</div>
		</section>
	</main>
}
