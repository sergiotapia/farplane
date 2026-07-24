Feature: Auth session
  Critical path: password setup, session cookie, login, and logout.

  Scenario: Owner setup creates a session and /me works
    Given a clean Farplane API
    When I set up an organization "Acme Co" as "owner@example.com" with password "password1"
    Then the response status is 201
    And I have a session cookie
    When I get "/api/v1/me"
    Then the response status is 200
    And the JSON field "organization.role" is "owner"

  Scenario: Login and logout round trip
    Given a clean Farplane API
    And an organization "Acme Co" owned by "owner@example.com" with password "password1"
    When I log out
    Then the response status is 204
    When I get "/api/v1/me"
    Then the response status is 401
    When I log in as "owner@example.com" with password "password1"
    Then the response status is 200
    And I have a session cookie
    When I get "/api/v1/me"
    Then the response status is 200
