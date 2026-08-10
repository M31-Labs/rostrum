package agenda

import "m31labs.dev/gosx/browser"

func SessionMoveForm(props any) Node {
	return <ActionForm class="move-form" actionName="moveSession">
		<input type="hidden" name="csrf_token" value={csrf.token}></input>
		<input type="hidden" name="session_id" value={props.id}></input>
		<div class="move-form-grid">
			<label>
				<span>Time</span>
				<select name="starts_at">
					<Each of={data.times} as="slot">
						<option value={slot.value} selected={slot.value == props.start}>{slot.label}</option>
					</Each>
				</select>
			</label>
			<label>
				<span>Room</span>
				<select name="room_id">
					<Each of={data.rooms} as="room">
						<option value={room.id} selected={room.id == props.roomID}>{room.name}</option>
					</Each>
				</select>
			</label>
			<label>
				<span>Track</span>
				<select name="track_id">
					<Each of={data.tracks} as="track">
						<option value={track.id} selected={track.id == props.trackID}>{track.name}</option>
					</Each>
				</select>
			</label>
		</div>
		<p class="form-error" data-gosx-field-error="starts_at" aria-live="polite"></p>
		<p class="form-status" role="status" aria-live="polite">{action.message}</p>
		<button class="button button-compact" type="submit">Check & move</button>
	</ActionForm>
}

//gosx:island
func AgendaBoard(props any) Node {
	draggedID := signal.New("")
	draggedTrack := signal.New("")
	draggedTitle := signal.New("")
	dropRoom := signal.New("")
	dropStart := signal.New("")
	overRoom := signal.New("")
	overStart := signal.New("")
	status := signal.New("")
	startDrag := func() {
		draggedID.Set(eventData != "" ? eventData : data["sessionId"])
		draggedTrack.Set(data["trackId"])
		draggedTitle.Set(data["sessionTitle"])
		status.Set("Picked up " + data["sessionTitle"] + ". Choose a room and time.")
	}
	endDrag := func() {
		draggedID.Set("")
		draggedTrack.Set("")
		draggedTitle.Set("")
		overRoom.Set("")
		overStart.Set("")
	}
	dragOver := func() {
		browser.PreventDefault(draggedID.Get() != "")
		overRoom.Set(draggedID.Get() != "" ? data["dropRoom"] : overRoom.Get())
		overStart.Set(draggedID.Get() != "" ? data["dropStart"] : overStart.Get())
	}
	dragLeave := func() {
		overRoom.Set(overRoom.Get() == data["dropRoom"] ? "" : overRoom.Get())
		overStart.Set(overStart.Get() == data["dropStart"] ? "" : overStart.Get())
	}
	drop := func() {
		browser.PreventDefault(draggedID.Get() != "")
		dropRoom.Set(draggedID.Get() != "" ? data["dropRoom"] : dropRoom.Get())
		dropStart.Set(draggedID.Get() != "" ? data["dropStart"] : dropStart.Get())
		status.Set(draggedID.Get() != "" ? "Moving " + draggedTitle.Get() + "…" : status.Get())
		overRoom.Set("")
		overStart.Set("")
		browser.Submit(draggedID.Get() != "", "#agenda-drag-form")
	}
	return <div class="agenda-board-island">
		<aside class="agenda-bank" aria-label="Unscheduled sessions">
			<header class="agenda-bank-head">
				<h2>Unscheduled</h2>
				<p>
					Accepted sessions waiting for a room and time. Drag a card onto the board to place it.
				</p>
			</header>
			<div class="agenda-bank-list">
				<Each of={props.bank} as="session">
					<article
						class={"agenda-card agenda-bank-card track-" + session.trackTone}
						draggable="true"
						aria-grabbed={draggedID.Get() == session.id}
						data-gosx-event-value={session.id}
						data-session-id={session.id}
						data-session-title={session.title}
						data-track-id={session.trackID}
						onDragStart={startDrag}
						onDragEnd={endDrag}
					>
						<div class="agenda-card-top">
							<span class="mono">{session.time}</span>
							<span class={"status-pill status-" + session.tone}>{session.status}</span>
						</div>
						<h3>{session.title}</h3>
						<p>{session.speakers}</p>
					</article>
				</Each>
				<p class="agenda-bank-empty" hidden={!props.bankEmpty}>
					Nothing waiting. Accepted talks land here until you place them.
				</p>
			</div>
		</aside>
		<section class="agenda-board" aria-label={props.date}>
			<header class="agenda-board-head">
				<span>{props.date}</span>
				<Each of={props.rooms} as="room">
					<div>
						<strong>{room.name}</strong>
						<small>{room.capacity}</small>
					</div>
				</Each>
			</header>
			<Each of={props.slots} as="slot">
				<div class="agenda-slot-row">
					<time class="agenda-time">{slot.label}</time>
					<Each of={slot.cells} as="cell">
						<div
							class={"agenda-cell" + (overRoom.Get() == cell.roomID && overStart.Get() == cell.start ? " drag-over" : "")}
							data-drop-room={cell.roomID}
							data-drop-start={cell.start}
							aria-label={"Drop into " + cell.roomName + " at " + slot.label}
							onDragOver={dragOver}
							onDragLeave={dragLeave}
							onDrop={drop}
						>
							<Each of={cell.sessions} as="session">
								<article
									class={"agenda-card track-" + session.trackTone}
									draggable="true"
									aria-grabbed={draggedID.Get() == session.id}
									data-gosx-event-value={session.id}
									data-session-id={session.id}
									data-session-title={session.title}
									data-track-id={session.trackID}
									onDragStart={startDrag}
									onDragEnd={endDrag}
								>
									<div class="agenda-card-top">
										<span class="mono">{session.time}</span>
										<span class={"status-pill status-" + session.tone}>{session.status}</span>
									</div>
									<h3>{session.title}</h3>
									<p>{session.speakers}</p>
									<details class="agenda-card-detail">
										<summary aria-label={"More about " + session.title}>⋯</summary>
										<div class="agenda-card-popover">
											<dl>
												<div>
													<dt>Room</dt>
													<dd>{session.room}</dd>
												</div>
												<div>
													<dt>Track</dt>
													<dd>{session.track}</dd>
												</div>
											</dl>
											<form
												class="agenda-unschedule-form"
												method="post"
												action={props.unscheduleAction}
												data-gosx-form
												data-gosx-form-mode="post"
												data-gosx-form-state="idle"
												data-gosx-enhance="form"
												data-gosx-enhance-layer="bootstrap"
												data-gosx-fallback="native-form"
											>
												<input type="hidden" name="csrf_token" value={props.csrf}></input>
												<input type="hidden" name="session_id" value={session.id}></input>
												<input type="hidden" name="day" value={props.day}></input>
												<button class="button button-compact" type="submit">Return to bank</button>
											</form>
										</div>
									</details>
								</article>
							</Each>
							<span class="drop-hint">Drop here</span>
						</div>
					</Each>
				</div>
			</Each>
		</section>
		<form
			id="agenda-drag-form"
			class="agenda-drag-form"
			method="post"
			action={props.action}
			data-gosx-form
			data-gosx-form-mode="post"
			data-gosx-form-state="idle"
			data-gosx-enhance="form"
			data-gosx-enhance-layer="bootstrap"
			data-gosx-fallback="native-form"
		>
			<input type="hidden" name="csrf_token" value={props.csrf}></input>
			<input type="hidden" name="session_id" value={draggedID.Get()}></input>
			<input type="hidden" name="track_id" value={draggedTrack.Get()}></input>
			<input type="hidden" name="room_id" value={dropRoom.Get()}></input>
			<input type="hidden" name="starts_at" value={dropStart.Get()}></input>
			<p class="form-error agenda-drag-error" data-gosx-field-error="starts_at" aria-live="polite"></p>
			<p class="form-status agenda-drag-status" role="alert" aria-live="assertive" tabindex="-1">{props.actionMessage}</p>
		</form>
		<p class="sr-only" role="status" aria-live="polite">{status.Get()}</p>
	</div>
}

func AgendaListRow(props any) Node {
	return <article class="agenda-list-row">
		<div class="agenda-list-time">
			<strong class="mono">{props.time}</strong>
			<small>{props.date}</small>
		</div>
		<div class="agenda-list-title">
			<h3>{props.title}</h3>
			<p>{props.speakers}</p>
		</div>
		<div class="agenda-list-place">
			<strong>{props.room}</strong>
			<small>{props.track}</small>
		</div>
		<span class={"status-pill status-" + props.tone}>{props.status}</span>
		<details>
			<summary>Move</summary>
			<SessionMoveForm {...props}></SessionMoveForm>
		</details>
	</article>
}

func Page() Node {
	return <main id="main-content" class="workspace-page agenda-page" data-section={data.section}>
		<header class="workspace-header">
			<div>
				<p class="eyebrow">Conflict-aware scheduling</p>
				<h1>Agenda</h1>
				<p>
					{data.sessionLabel}
					·
					{data.hardLabel}
					·
					{data.warnLabel + "."}
				</p>
			</div>
			<div class="workspace-header-actions">
				<a class="button" href={data.workspace.publicAgendaHref} data-gosx-link>Preview public agenda</a>
				<ActionForm actionName="publishAgenda">
					<input type="hidden" name="csrf_token" value={csrf.token}></input>
					<p class="form-error" data-gosx-field-error="agenda" aria-live="polite"></p>
					<p class="form-status" role="status" aria-live="polite">{action.message}</p>
					<button class="button button-primary" type="submit" disabled={!data.publishable}>Publish agenda</button>
				</ActionForm>
			</div>
		</header>
		<p class="flash-message">
			{flash.notice}
			{action.message}
		</p>
		<div class="agenda-toolbar">
			<nav class="view-switcher" aria-label="Agenda view">
				<Each of={data.views} as="view">
					<a class={view.class} href={view.href} data-gosx-link>{view.label}</a>
				</Each>
			</nav>
			<nav class="day-switcher" aria-label="Agenda day">
				<Each of={data.days} as="day">
					<a class={day.class} href={day.href} data-gosx-link>{day.label}</a>
				</Each>
			</nav>
			<p>
				<span aria-hidden="true">↕</span>
				Drag a card or open its keyboard-accessible move controls.
			</p>
		</div>
		<If cond={data.boardView}>
			<AgendaBoard
				date={data.date}
				day={data.dayKey}
				rooms={data.rooms}
				slots={data.slots}
				bank={data.bank}
				bankEmpty={data.bankEmpty}
				action={actionPath("moveSession")}
				unscheduleAction={actionPath("unscheduleSession")}
				csrf={csrf.token}
				actionMessage={"" + action.message}
			></AgendaBoard>
			<details class="agenda-keyboard-moves">
				<summary>Move a session without drag and drop</summary>
				<div class="agenda-list">
					<Each of={data.moveSessions} as="session">
						<AgendaListRow {...session}></AgendaListRow>
					</Each>
				</div>
			</details>
		</If>
		<If cond={!data.boardView}>
			<section class="agenda-list-view" aria-label={data.view + " agenda view"}>
				<Each of={data.groups} as="group">
					<article class="agenda-group">
						<header>
							<div>
								<p class="panel-kicker">{group.detail}</p>
								<h2>{group.label}</h2>
							</div>
							<span class="mono">
								{group.count}
								sessions
							</span>
						</header>
						<If cond={group.empty}>
							<p class="agenda-empty">No sessions scheduled.</p>
						</If>
						<If cond={!group.empty}>
							<div class="agenda-list">
								<Each of={group.sessions} as="session">
									<AgendaListRow {...session}></AgendaListRow>
								</Each>
							</div>
						</If>
					</article>
				</Each>
			</section>
		</If>
		<section class="panel conflict-panel">
			<header class="panel-header">
				<div>
					<p class="panel-kicker">Scheduling rules</p>
					<h2>Conflict inspector</h2>
				</div>
				<span>
					{data.hardCount}
					blocking ·
					{data.warnCount}
					warnings
				</span>
			</header>
			<div class="conflict-list">
				<Each of={data.conflicts} as="conflict">
					<article class={"conflict-card conflict-" + conflict.tone}>
						<div class="conflict-icon" aria-hidden="true">{conflict.kind}</div>
						<div>
							<div class="conflict-title">
								<strong>{conflict.sessions}</strong>
								<span class={"status-pill status-" + conflict.tone}>{conflict.severity}</span>
							</div>
							<p>
								{conflict.message}
								{conflict.reason}
							</p>
							<details>
								<summary>Decision trace</summary>
								<code>{conflict.trace}</code>
							</details>
						</div>
						<span class="policy-rule">{conflict.rule}</span>
					</article>
				</Each>
			</div>
		</section>
	</main>
}
