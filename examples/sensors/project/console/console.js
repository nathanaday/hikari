// Lists the cloud's incidents, newest first, and refreshes every 5 seconds.
async function load() {
  const status = document.getElementById('status')
  try {
    const res = await fetch('/api/incidents')
    const incidents = (await res.json()).reverse()
    const body = document.querySelector('#incidents tbody')
    body.replaceChildren(
      ...incidents.map((i) => {
        const row = document.createElement('tr')
        for (const v of [i.sensor, i.value, i.threshold, new Date(i.at * 1000).toLocaleString()]) {
          const cell = document.createElement('td')
          cell.textContent = String(v)
          row.append(cell)
        }
        return row
      }),
    )
    status.textContent = incidents.length ? `${incidents.length} incidents` : 'No incidents.'
  } catch (e) {
    status.textContent = 'The cloud cannot be reached.'
  }
}
load()
setInterval(load, 5000)
