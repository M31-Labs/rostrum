package public

//gosx:island
func PublicItinerary(props any) Node {
	saved := signal.NewShared("$rostrumItinerary", []string{})
	status := signal.New("")
	toggle := func() {
		status.Set(saved.Get().contains(data["id"]) ? data["title"] + " removed from your itinerary." : data["title"] + " added to your itinerary.")
		saved.Set(saved.Get().contains(data["id"]) ? saved.Get().filter(func(item){ return item != data["id"] }) : saved.Get().append(data["id"]))
	}
	clear := func() {
		saved.Set(saved.Get().filter(func(item){ return false }))
		status.Set("Itinerary cleared.")
	}
	return <div class="public-itinerary">
		<div class="public-agenda-list">
			<Each of={props.sessions} as="session">
				<article
					class={"public-session track-" + session.trackTone + (saved.Get().contains(session.id) ? " saved" : "")}
					data-session={session.id}
				>
					<time>
						<strong>{session.timeShort}</strong>
						<span>{session.time}</span>
					</time>
					<div class="public-session-copy">
						<div>
							<span>{session.track}</span>
							<span>{session.format}</span>
						</div>
						<h2>{session.title}</h2>
						<p>{session.description}</p>
						<footer>
							<span class="avatar avatar-small">{session.speakerInitials}</span>
							<strong>{session.speakers}</strong>
							<span>{session.room}</span>
						</footer>
					</div>
					<button
						class="itinerary-button"
						type="button"
						data-id={session.id}
						data-title={session.title}
						aria-pressed={saved.Get().contains(session.id)}
						onClick={toggle}
					>
						<span aria-hidden="true">
							{saved.Get().contains(session.id) ? "✓" : "+"}
						</span>
						<b>
							{saved.Get().contains(session.id) ? "Saved" : "Add"}
						</b>
					</button>
				</article>
			</Each>
		</div>
		<div class="itinerary-dock" hidden={saved.Get().length == 0}>
			<span>
				<strong>{saved.Get().length}</strong>
				saved
			</span>
			<button type="button" onClick={clear}>Clear itinerary</button>
		</div>
		<p class="sr-only" role="status" aria-live="polite">{status.Get()}</p>
	</div>
}

func Page() Node {
	return <main id="main-content" class={data.pageClass}>
		<header class="public-event-hero">
			<div>
				<p class="eyebrow">
					Program ·
					{data.event.dates}
				</p>
				<h1>{data.event.name}</h1>
				<p>{data.event.theme}</p>
			</div>
			<nav aria-label="Event">
				<a class="active" href={data.event.agendaURL} data-gosx-link>Agenda</a>
				<a href={data.event.speakersURL} data-gosx-link>Speakers</a>
			</nav>
		</header>
		<section class="public-agenda-intro">
			<div>
				<h2>Build your day.</h2>
				<p>
					{data.sessionCount}
					sessions across four tracks. Add any session to a personal itinerary stored on this device.
				</p>
			</div>
			<div class="track-legend">
				<Each of={data.tracks} as="track">
					<span class={"track-" + track.tone}>
						<i></i>
						{track.name}
					</span>
				</Each>
			</div>
		</section>
		<PublicItinerary sessions={data.sessions}></PublicItinerary>
	</main>
}
