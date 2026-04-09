-- Fix Greenhouse companies with wrong board tokens
UPDATE job_companies SET careers_url = 'https://boards.greenhouse.io/dbtlabsinc' WHERE id = 12055;
UPDATE job_companies SET careers_url = 'https://boards.greenhouse.io/weave' WHERE id = 12022;
UPDATE job_companies SET careers_url = 'https://boards.greenhouse.io/unity3d' WHERE id = 11994;
UPDATE job_companies SET careers_url = 'https://boards.greenhouse.io/togetherai' WHERE id = 11969;
UPDATE job_companies SET careers_url = 'https://boards.greenhouse.io/toast' WHERE id = 11968;
UPDATE job_companies SET careers_url = 'https://boards.greenhouse.io/shifttechnology' WHERE id = 11875;
UPDATE job_companies SET careers_url = 'https://boards.greenhouse.io/scaleai' WHERE id = 11857;
UPDATE job_companies SET careers_url = 'https://boards.greenhouse.io/rampnetwork' WHERE id = 11808;
UPDATE job_companies SET careers_url = 'https://boards.greenhouse.io/processstreet' WHERE id = 11787;
UPDATE job_companies SET careers_url = 'https://boards.greenhouse.io/platformsh' WHERE id = 11776;
UPDATE job_companies SET careers_url = 'https://boards.greenhouse.io/planetlabs' WHERE id = 11774;
UPDATE job_companies SET careers_url = 'https://boards.greenhouse.io/peloton' WHERE id = 11754;
UPDATE job_companies SET careers_url = 'https://boards.greenhouse.io/pantherlabs' WHERE id = 11744;
UPDATE job_companies SET careers_url = 'https://boards.greenhouse.io/oscar' WHERE id = 11733;
UPDATE job_companies SET careers_url = 'https://boards.greenhouse.io/lucidsoftware' WHERE id = 11646;
UPDATE job_companies SET careers_url = 'https://boards.greenhouse.io/grafanalabs' WHERE id = 11508;
UPDATE job_companies SET careers_url = 'https://boards.greenhouse.io/googlefiber' WHERE id = 11506;
UPDATE job_companies SET careers_url = 'https://boards.greenhouse.io/nextdoor' WHERE id = 11497;
UPDATE job_companies SET careers_url = 'https://boards.greenhouse.io/datadog' WHERE id = 11394;
UPDATE job_companies SET careers_url = 'https://boards.greenhouse.io/boxinc' WHERE id = 11280;
UPDATE job_companies SET careers_url = 'https://boards.greenhouse.io/aurorainnovation' WHERE id = 11233;
UPDATE job_companies SET careers_url = 'https://boards.greenhouse.io/daangn' WHERE id = 11600;

-- Fix Ashby companies (slug correction)
UPDATE job_companies SET careers_url = 'https://jobs.ashbyhq.com/zip' WHERE id = 12046;

-- Reset failure counts so they get retried
UPDATE job_companies SET sync_failure_count = 0, last_sync_error = '', last_sync_status = 'pending', is_active = true
WHERE id IN (12055,12022,11994,11969,11968,11875,11857,11808,11787,11776,11774,11754,11744,11733,11646,11508,11506,11497,11394,11280,11233,11600,12046,11851,11767,11619,11383);

-- Deactivate companies that no longer exist on their ATS
UPDATE job_companies SET is_active = false
WHERE id IN (12016,11950,11923,11882,11757,11708,11663,11612,11590,11540,11538,11500,11486,11432,11335,11275,11240,11221,11219,11177,11512);
