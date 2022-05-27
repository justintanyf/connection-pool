-- users = {} // global user account pool, randomly pick one for sending login req , we assume all user have same password
-- threads = {}

function setup(thread)  -- build user pool
   -- build your 200 users
    --for i = 0, 200 do
    --    table.insert(users, i)
    --end
   wrk.method = "POST"
   wrk.headers["Content-Type"] = "application/x-www-form-urlencoded"
   -- table.insert(threads,thread)
   -- thread:set("users",users) -- let wrk can access user pool
end

function request()
    --random pick a user account
   acc = math.random(199)
   return wrk.format(
    "POST",
    "/login/",
    {["Content-Type"] = "application/x-www-form-urlencoded"},
    "account=" .. acc .. "&password=test_password"
   )
end
