package public

func Page() Node {
	return <main id="main-content" class={data.pageClass}>
		<header class="public-event-hero">
			<div>
				<p class="eyebrow">
					People ·
					{data.event.dates}
				</p>
				<h1>{data.event.name}</h1>
				<p>
					{data.speakerCount}
					builders sharing work from the field.
				</p>
			</div>
			<nav aria-label="Event">
				<a href={data.event.agendaURL} data-gosx-link>Agenda</a>
				<a class="active" href={data.event.speakersURL} data-gosx-link>Speakers</a>
			</nav>
		</header>
		<section class="public-speaker-grid">
			<Each of={data.speakers} as="speaker">
				<article class="public-speaker-card">
					<div class="speaker-portrait">
						<span>{speaker.initials}</span>
						<i aria-hidden="true"></i>
					</div>
					<div class="public-speaker-copy">
						<p class="panel-kicker">{speaker.city}</p>
						<h2>{speaker.name}</h2>
						<p class="speaker-role">
							{speaker.role}
							·
							{speaker.company}
						</p>
						<p>{speaker.biography}</p>
						<div class="speaker-talks">
							<Each of={speaker.talks} as="talk">
								<article>
									<strong>{talk.title}</strong>
									<span>
										{talk.time}
										·
										{talk.room}
									</span>
								</article>
							</Each>
						</div>
						<footer>
							<If cond={speaker.website != ""}>
								<a href={speaker.website} target="_blank" rel="noreferrer">Website ↗</a>
							</If>
							<If cond={speaker.linkedin != ""}>
								<a href={speaker.linkedin} target="_blank" rel="noreferrer">LinkedIn ↗</a>
							</If>
						</footer>
					</div>
				</article>
			</Each>
		</section>
	</main>
}
