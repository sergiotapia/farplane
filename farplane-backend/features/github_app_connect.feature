Feature: GitHub App connect
  Critical path: create the Farplane GitHub App via the manifest flow.

  Scenario: Owner creates a GitHub App from the manifest
    Given a clean Farplane API with a public API base URL
    And an organization "Acme" owned by "owner@example.com" with password "password1"
    And a fake GitHub App manifest converter
    When I start the GitHub App manifest flow
    Then the response status is 200
    And the JSON field "action" is "https://github.com/settings/apps/new"
    When I complete the GitHub App manifest callback with code "manifest-code"
    Then the response status is 302
    And the redirect location contains "github=app_created"
    When I get "/api/v1/github/installations"
    Then the response status is 200
    And the JSON field "configured" is true
