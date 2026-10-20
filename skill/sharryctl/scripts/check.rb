#!/usr/bin/env ruby
# frozen_string_literal: true

# Reports sharryctl readiness as JSON: binary, tenants and logged-in profiles.
# Never prints secrets.

require "json"
require "open3"

def run(*args)
  out, _err, status = Open3.capture3("sharryctl", *args)
  [out, status.success?]
rescue Errno::ENOENT
  [nil, false]
end

version, ok = run("--version")
unless ok
  puts JSON.pretty_generate(installed: false, hint: "brew install alhafoudh/sharryctl/sharryctl")
  exit 1
end

tenants_json, = run("--json", "config", "tenants", "list")
tenants = JSON.parse(tenants_json.to_s.strip.empty? ? "[]" : tenants_json) rescue []
tenants ||= []

status_json, status_ok = run("--json", "auth", "status")
status = status_ok ? JSON.parse(status_json) : nil

puts JSON.pretty_generate(
  installed: true,
  version: version.strip,
  tenants: tenants,
  active_session: status,
  next_step:
    if tenants.empty? then "configure a tenant: sharryctl config tenants add <slug>"
    elsif status.nil? then "log in: scripts/login.rb request <email>"
    else "ready"
    end
)
