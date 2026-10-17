package main

// defaultRooms are created at startup when they do not exist yet.
// Change them in the rooms table afterwards. Your changes are kept.
var defaultRooms = []roomRow{
	{Slug: "lobby", Name: "General Chat", Category: "General", Tags: "Vibes,Friends,Random", Sort: 10},
	{Slug: "abuja", Name: "Abuja", Category: "Cities", Tags: "Abuja,Meetups,Gist", Sort: 20},
	{Slug: "lagos", Name: "Lagos", Category: "Cities", Tags: "Lagos,Hustle,Gist", Sort: 30},
	{Slug: "campus", Name: "Campus Gist", Category: "Education", Tags: "Students,Gist,Study", Sort: 40},
	{Slug: "football", Name: "Sport", Category: "Sports", Tags: "Football,Basketball,Athletics", Sort: 50},
	{Slug: "music", Name: "Music Heads", Category: "Entertainment", Tags: "Afrobeats,Hip hop,Playlists", Sort: 60},
	{Slug: "tech", Name: "Tech Hustle", Category: "Tech", Tags: "Tech,Business,Innovation", Sort: 70},
	{Slug: "side-hustle", Name: "Side Hustle", Category: "Business", Tags: "Freelance,Skills,Income", Sort: 80},
	{Slug: "creators", Name: "Creators Hub", Category: "Creators", Tags: "Content,Growth,Collab", Sort: 90},
	{Slug: "elite-lounge", Name: "Elite Lounge", Category: "Members", Tags: "Elite,Legends,Private", MinTier: 3, Sort: 100},
}
