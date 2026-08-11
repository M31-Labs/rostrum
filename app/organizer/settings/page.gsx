package settings

func SettingError(props any) Node {
	return <p class="form-error" id={props.id} aria-live="polite">{props.message}</p>
}

func SettingsFields(props any) Node {
	return <fragment>
		<label>
			<span>Event name</span>
			<input name="name" value={props.name} aria-describedby="settings-name-error" required></input>
			<SettingError id="settings-name-error" message={props.nameError}></SettingError>
		</label>
		<div class="form-grid-two">
			<label>
				<span>Slug</span>
				<input name="slug" value={props.slug} aria-describedby="settings-slug-error" required></input>
				<SettingError id="settings-slug-error" message={props.slugError}></SettingError>
			</label>
			<label>
				<span>Event type</span>
				<input name="type" value={props.type}></input>
			</label>
		</div>
		<label>
			<span>Website URL</span>
			<input type="url" name="website" value={props.website}></input>
		</label>
		<label>
			<span>Location</span>
			<input name="location" value={props.location} aria-describedby="settings-location-error" required></input>
			<SettingError id="settings-location-error" message={props.locationError}></SettingError>
		</label>
		<label>
			<span>Timezone</span>
			<input name="timezone" value={props.timezone} aria-describedby="settings-timezone-error" required></input>
			<SettingError id="settings-timezone-error" message={props.timezoneError}></SettingError>
		</label>
		<div class="form-grid-two">
			<label>
				<span>Starts</span>
				<input
					type="datetime-local"
					name="starts_at"
					value={props.starts}
					aria-describedby="settings-starts-error"
					required
				></input>
				<SettingError id="settings-starts-error" message={props.startsError}></SettingError>
			</label>
			<label>
				<span>Ends</span>
				<input
					type="datetime-local"
					name="ends_at"
					value={props.ends}
					aria-describedby="settings-ends-error"
					required
				></input>
				<SettingError id="settings-ends-error" message={props.endsError}></SettingError>
			</label>
		</div>
		<label>
			<span>Theme</span>
			<input name="theme" value={props.theme}></input>
		</label>
		<label>
			<span>Description</span>
			<textarea name="description">{props.description}</textarea>
		</label>
	</fragment>
}

func Page() Node {
	return <main id="main-content" class="workspace-page" data-section={data.section}>
		<header class="workspace-header">
			<div>
				<p class="eyebrow">Event configuration</p>
				<h1>Settings</h1>
				<p>
					Identity, location, timezone, dates, tracks, rooms, and public theme live in one event record.
				</p>
			</div>
			<div class="workspace-header-actions">
				<a class="button" href={"/public/" + data.event.slug + "/agenda"} data-gosx-link>Preview event</a>
			</div>
		</header>
		<p class="flash-message">{flash.notice}</p>
		<div class="settings-grid">
			<ActionForm class="panel settings-form" actionName="saveEvent">
				<input type="hidden" name="csrf_token" value={csrf.token}></input>
				<p class="form-status" role="alert" aria-live="assertive">{actions.saveEvent.message}</p>
				<header class="panel-header">
					<div>
						<p class="panel-kicker">Core details</p>
						<h2>Event identity</h2>
					</div>
				</header>
				<If cond={actions.saveEvent.name != ""}>
					<SettingsFields
						name={actions.saveEvent.values.name}
						slug={actions.saveEvent.values.slug}
						type={actions.saveEvent.values.type}
						website={actions.saveEvent.values.website}
						location={actions.saveEvent.values.location}
						timezone={actions.saveEvent.values.timezone}
						starts={actions.saveEvent.values.starts_at}
						ends={actions.saveEvent.values.ends_at}
						theme={actions.saveEvent.values.theme}
						description={actions.saveEvent.values.description}
						nameError={actions.saveEvent.fieldErrors.name}
						slugError={actions.saveEvent.fieldErrors.slug}
						locationError={actions.saveEvent.fieldErrors.location}
						timezoneError={actions.saveEvent.fieldErrors.timezone}
						startsError={actions.saveEvent.fieldErrors.starts_at}
						endsError={actions.saveEvent.fieldErrors.ends_at}
					></SettingsFields>
				</If>
				<If cond={actions.saveEvent.name == ""}>
					<SettingsFields
						name={data.event.name}
						slug={data.event.slug}
						type={data.event.type}
						website={data.event.website}
						location={data.event.location}
						timezone={data.event.timezone}
						starts={data.event.starts}
						ends={data.event.ends}
						theme={data.event.theme}
						description={data.event.description}
						nameError=""
						slugError=""
						locationError=""
						timezoneError=""
						startsError=""
						endsError=""
					></SettingsFields>
				</If>
				<p class="form-note">
					Moving the event start date shifts every scheduled agenda item by the same calendar amount. Changing timezone preserves each session’s local wall-clock time.
				</p>
				<button class="button button-primary" type="submit">Save settings</button>
			</ActionForm>
			<aside class="settings-side">
				<section class="panel">
					<header class="panel-header">
						<div>
							<p class="panel-kicker">Program shape</p>
							<h2>Tracks</h2>
						</div>
						<span>{data.trackCount}</span>
					</header>
					<div class="settings-list">
						<Each of={data.tracks} as="track">
							<article>
								<i class={"track-dot track-" + track.Color}></i>
								<div>
									<strong>{track.Name}</strong>
									<small>{track.Description}</small>
								</div>
							</article>
						</Each>
					</div>
					<ActionForm class="settings-add-form" actionName="addTrack">
						<input type="hidden" name="csrf_token" value={csrf.token}></input>
						<p class="form-status" role="status" aria-live="polite">{actions.addTrack.message}</p>
						<label>
							<span>New track name</span>
							<input name="name" required></input>
							<p class="form-error" data-gosx-field-error="name" aria-live="polite"></p>
						</label>
						<div class="form-grid-two">
							<label>
								<span>Color</span>
								<select name="color" required>
									<option value="blue">Blue</option>
									<option value="teal">Teal</option>
									<option value="violet">Violet</option>
									<option value="ochre">Ochre</option>
								</select>
								<p class="form-error" data-gosx-field-error="color" aria-live="polite"></p>
							</label>
							<label>
								<span>Description</span>
								<input name="description" maxlength="500"></input>
								<p class="form-error" data-gosx-field-error="description" aria-live="polite"></p>
							</label>
						</div>
						<button class="button" type="submit">Add track</button>
					</ActionForm>
				</section>
				<section class="panel">
					<header class="panel-header">
						<div>
							<p class="panel-kicker">Venue</p>
							<h2>Rooms</h2>
						</div>
						<span>{data.roomCount}</span>
					</header>
					<div class="settings-list">
						<Each of={data.rooms} as="room">
							<article>
								<span class="mono">{room.Capacity}</span>
								<div>
									<strong>{room.Name}</strong>
									<small>seated capacity</small>
								</div>
							</article>
						</Each>
					</div>
					<ActionForm class="settings-add-form" actionName="addRoom">
						<input type="hidden" name="csrf_token" value={csrf.token}></input>
						<p class="form-status" role="status" aria-live="polite">{actions.addRoom.message}</p>
						<div class="form-grid-two">
							<label>
								<span>New room name</span>
								<input name="name" required></input>
								<p class="form-error" data-gosx-field-error="name" aria-live="polite"></p>
							</label>
							<label>
								<span>Seated capacity</span>
								<input type="number" name="capacity" min="1" max="100000" required></input>
								<p class="form-error" data-gosx-field-error="capacity" aria-live="polite"></p>
							</label>
						</div>
						<button class="button" type="submit">Add room</button>
					</ActionForm>
				</section>
				<section class="panel">
					<header class="panel-header">
						<div>
							<p class="panel-kicker">CFP routing</p>
							<h2>Categories</h2>
						</div>
						<span>{data.categoryCount}</span>
					</header>
					<div class="settings-list">
						<Each of={data.categories} as="category">
							<article>
								<span class="mono">{category.ID}</span>
								<div>
									<strong>{category.Name}</strong>
									<small>
										{category.OwnerName}
										·
										{category.OwnerEmail}
									</small>
								</div>
							</article>
						</Each>
					</div>
					<ActionForm class="settings-add-form" actionName="addCategory">
						<input type="hidden" name="csrf_token" value={csrf.token}></input>
						<p class="form-status" role="status" aria-live="polite">{actions.addCategory.message}</p>
						<label>
							<span>New category name</span>
							<input name="name" required></input>
							<p class="form-error" data-gosx-field-error="name" aria-live="polite"></p>
						</label>
						<div class="form-grid-two">
							<label>
								<span>Category owner</span>
								<input name="owner_name" required></input>
								<p class="form-error" data-gosx-field-error="owner_name" aria-live="polite"></p>
							</label>
							<label>
								<span>Owner email</span>
								<input type="email" name="owner_email" required></input>
								<p class="form-error" data-gosx-field-error="owner_email" aria-live="polite"></p>
							</label>
						</div>
						<label>
							<span>Program track</span>
							<select name="track_id" required>
								<option value="">Choose a track</option>
								<Each of={data.tracks} as="track">
									<option value={track.ID}>{track.Name}</option>
								</Each>
							</select>
							<p class="form-error" data-gosx-field-error="track_id" aria-live="polite"></p>
						</label>
						<p class="form-note">
							Until a policy names this category,
							<code>{data.categoryFallback.rule}</code>
							routes it to
							{data.categoryFallback.queue}
							on
							{data.categoryFallback.track}
							.
						</p>
						<button class="button" type="submit">Add category</button>
					</ActionForm>
				</section>
				<section class="panel">
					<header class="panel-header">
						<div>
							<p class="panel-kicker">Ownership</p>
							<h2>Export and restore</h2>
						</div>
					</header>
					<p>
						Download a checksummed workspace for a guided restore, or a full archive with uploads and the independent audit ledger.
					</p>
					<p class="settings-actions">
						<a class="button" href="/organizer/export/workspace.json">Download workspace</a>
						<a class="button" href="/organizer/export/archive.tar.gz">Download full archive</a>
					</p>
					<form
						class="settings-form workspace-import-form"
						method="post"
						action="/organizer/import/workspace"
						enctype="multipart/form-data"
					>
						<input type="hidden" name="csrf_token" value={csrf.token}></input>
						<label>
							<span>Restore workspace export</span>
							<input type="file" name="workspace" accept="application/json,.json" required></input>
						</label>
						<p class="form-note">
							A validated restore first creates a local backup. This host’s organizer principals, passkeys, and pending sign-in links stay in place.
						</p>
						<button class="button button-primary" type="submit">Restore workspace</button>
					</form>
				</section>
			</aside>
		</div>
	</main>
}
