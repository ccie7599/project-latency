**Linode Global Latency Map — Live Demo**

Built a real-time latency visualization tool that shows measured RTT between all 45 Linode compute regions worldwide. Deploys a lightweight probe in every region — core and distributed edge sites — forming a full mesh that continuously measures direct region-to-region latency. Data feeds into Prometheus and renders on a retro-styled network map.

**Link**: https://latency-demo.connected-cloud.io (token shared separately)

What you can do:
- **Filter by continent/country** or select individual regions — arcs show measured RTT with ms labels
- **Click any region** to see its latency to every other region, sorted fastest to slowest
- **Heatmap Matrix** — 45×45 grid with integer ms values, hover for details
- **AZ Pair Selector** — pick a primary region and see recommendations for sync replication (<10ms), async (<50ms), and DR pairs with distance + rationale
- **MY LOCATION** — enter coordinates or share browser location to see nearest regions by distance (client latency test is pending a firewall exception — will show actual measured RTT from your browser to each region once approved)

The demo runs through Akamai CDN with DS2 logging to ClickHouse. 45 Linode regions (34 core + 11 distributed/edge sites) across NA, SA, EU, APAC, Oceania, and Africa. Full mesh gives ~1,980 directional measurements updated every 10 seconds.

Feedback welcome — especially on the UI and any features that would make this more useful for customer conversations.
