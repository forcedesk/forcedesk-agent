<?php

namespace App\Http\Middleware;

use Closure;
use Illuminate\Http\Request;
use Symfony\Component\HttpFoundation\Response;

class ProxyMiddleware
{
    /**
     * Handle an incoming request.
     *
     * @param  \Closure(\Illuminate\Http\Request): (\Symfony\Component\HttpFoundation\Response)  $next
     */
    public function handle(Request $request, Closure $next): Response
    {

        /*
        if (config()->has('agentconfig.proxies.address') && config('agentconfig.proxies.address') !== null) {
            config(['http.proxy' => config('agentconfig.proxies.address')]);
            config(['https.proxy' => config('agentconfig.proxies.address')]);
        }
        */

        return $next($request);
    }
}
