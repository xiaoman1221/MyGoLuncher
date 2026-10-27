package app

import "testing"

func TestRankResourceProjectsPrefersExactProject(t *testing.T) {
	projects := []ResourceProject{
		{Title: "Farm Better with Hoes - Farmers Delight Compatibility", Slug: "better-farming-with-hoes", Downloads: 50_000_000},
		{Title: "Farmers Delight Kacchi", Slug: "farmers-delight-kacchi", Downloads: 2_000_000},
		{Title: "Farmer's Delight", Slug: "farmers-delight", Downloads: 10_000_000},
	}
	rankResourceProjects(projects, "farmers delight")
	if projects[0].Slug != "farmers-delight" {
		t.Fatalf("expected the exact project first, got %#v", projects[0])
	}
}

func TestRankResourceProjectsHandlesWordOrder(t *testing.T) {
	projects := []ResourceProject{
		{Title: "Roughly Enough Items (REI)", Slug: "rei", Downloads: 80_000_000},
		{Title: "Just Enough Items (JEI)", Slug: "jei", Downloads: 5_000_000},
	}
	rankResourceProjects(projects, "jei just enough items")
	if projects[0].Slug != "jei" {
		t.Fatalf("expected JEI first, got %#v", projects[0])
	}
}

func TestRankResourceProjectsKeepsInventoryOrderForBlankQuery(t *testing.T) {
	projects := []ResourceProject{{Title: "B"}, {Title: "A"}}
	rankResourceProjects(projects, "  ")
	if projects[0].Title != "B" {
		t.Fatalf("blank query should not reorder: %#v", projects)
	}
}
