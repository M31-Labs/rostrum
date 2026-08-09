package embeds

import "m31labs.dev/gosx/browser"

//gosx:island
func EmbedClipboard(props any) Node {
	label := signal.New("Copy code")
	status := signal.New("")
	copy := func() {
		label.Set(browser.ClipboardWrite(code) ? "Copied" : "Copy failed")
		status.Set(label.Get() == "Copied" ? "Embed code copied to the clipboard." : "Could not copy the embed code. Select the code and copy it manually.")
	}
	return <div class="embed-code-block">
		<pre tabindex="0">
			<code>{code}</code>
		</pre>
		<button class="button button-compact" type="button" onClick={copy}>{label.Get()}</button>
		<span class="sr-only" role="status" aria-live="polite">{status.Get()}</span>
	</div>
}

func Page() Node {
	return <main id="main-content" class="workspace-page" data-section={data.section}>
		<header class="workspace-header">
			<div>
				<p class="eyebrow">Publish everywhere</p>
				<h1>Embeds</h1>
				<p>
					Responsive agenda and speaker surfaces inherit the same canonical records as the portal and API.
				</p>
			</div>
			<div class="workspace-header-actions">
				<a class="button" href={data.event.agendaURL} data-gosx-link>Open agenda</a>
				<a class="button button-primary" href={data.event.speakersURL} data-gosx-link>Open gallery</a>
			</div>
		</header>
		<section class="embed-grid">
			<article class="embed-preview">
				<header>
					<div>
						<p class="panel-kicker">Mobile agenda</p>
						<h2>Itinerary-ready schedule</h2>
					</div>
					<span class="status-pill status-positive">Live preview</span>
				</header>
				<div class="device-frame">
					<iframe src={data.agenda.src} title="Agenda embed preview" loading="lazy"></iframe>
				</div>
				<details class="embed-code-disclosure">
					<summary>Embed code</summary>
					<EmbedClipboard code={data.agenda.code}></EmbedClipboard>
				</details>
			</article>
			<article class="embed-preview">
				<header>
					<div>
						<p class="panel-kicker">Speaker gallery</p>
						<h2>Profiles linked to sessions</h2>
					</div>
					<span class="status-pill status-positive">Live preview</span>
				</header>
				<div class="device-frame">
					<iframe src={data.speakers.src} title="Speaker gallery embed preview" loading="lazy"></iframe>
				</div>
				<details class="embed-code-disclosure">
					<summary>Embed code</summary>
					<EmbedClipboard code={data.speakers.code}></EmbedClipboard>
				</details>
			</article>
		</section>
		<section class="panel embed-api">
			<header class="panel-header">
				<div>
					<p class="panel-kicker">Headless option</p>
					<h2>Use the API instead</h2>
				</div>
				<span class="mono">JSON</span>
			</header>
			<p>
				Build a fully custom surface from the same event, session, speaker, and task graph.
			</p>
			<a class="button button-compact" href={data.api}>Open workspace endpoint</a>
		</section>
	</main>
}
